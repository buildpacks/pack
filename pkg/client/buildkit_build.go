package client

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/lifecycle/platform/files"
	"github.com/docker/docker/api/types"
	// gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/buildkit/executor"
	"github.com/buildpacks/pack/internal/buildkit/state"
	internalConfig "github.com/buildpacks/pack/internal/config"
	pname "github.com/buildpacks/pack/internal/name"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/termui"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	v02 "github.com/buildpacks/pack/pkg/project/v02"
)

func (c *Client) BuildWithBuildkit(ctx context.Context, opts BuildOptions) error {
	var pathsConfig layoutPathConfig

	imageRef, err := c.parseReference(opts)
	if err != nil {
		return errors.Wrapf(err, "invalid image name '%s'", opts.Image)
	}
	imgRegistry := imageRef.Context().RegistryStr()
	imageName := imageRef.Name()

	if opts.Layout() {
		pathsConfig, err = c.processLayoutPath(opts.LayoutConfig.InputImage, opts.LayoutConfig.PreviousInputImage)
		if err != nil {
			if opts.LayoutConfig.PreviousInputImage != nil {
				return errors.Wrapf(err, "invalid layout paths image name '%s' or previous-image name '%s'", opts.LayoutConfig.InputImage.Name(),
					opts.LayoutConfig.PreviousInputImage.Name())
			}
			return errors.Wrapf(err, "invalid layout paths image name '%s'", opts.LayoutConfig.InputImage.Name())
		}
	}

	appPath, err := c.processAppPath(opts.AppPath)
	if err != nil {
		return errors.Wrapf(err, "invalid app path '%s'", opts.AppPath)
	}

	proxyConfig := c.processProxyConfig(opts.ProxyConfig)

	builderRef, err := c.processBuilderName(opts.Builder)
	if err != nil {
		return errors.Wrapf(err, "invalid builder '%s'", opts.Builder)
	}

	var buidlerState *state.State = state.Remote("busybox:latest")
	// switch opts.PullPolicy {
	// case image.PullNever:
	// 	buidlerState = state.Local(builderRef.Name())
	// case image.PullAlways:
	// 	buidlerState = state.Remote(builderRef.Name(), llb.MarkImageInternal)
	// default:
	// 	buidlerState = state.Local(builderRef.Name())
	// 	// if err := state.Validate(ctx, llb.NewConstraints(llb.LocalUniqueID(identity.NewID()))); err != nil {
	// 	// 	// lets not validate llb.Image
	// 	// 	state = llb.Image(builderRef.Name())
	// 	// }
	// }

	// NewClient, err := client.New(ctx, "") // use default address
	// if err != nil {
	// 	return err
	// }
	
	var pforms = make([]ocispecs.Platform, 0)
	for _, t := range opts.Targets {
		t.Range(ctx, func(target dist.Target) error {
			pforms = append(pforms, ocispecs.Platform{
				OS: target.OS,
				Architecture: target.Arch,
				Variant: target.ArchVariant,
			})
			return nil
		})
	}

	// bkBldr, err := NewBuilder(imageName, builderRef.Name(), c, pforms, opts)
	// if err != nil {
	// 	return err
	// }

	// bkBldr.State = buidlerState
	// // def, err := buidlerState.State().Marshal(ctx)
	// // if err != nil {
	// // 	return err
	// // }

	// buildkitCtx := namespaces.WithNamespace(ctx, "buildkit")
	// _, err = NewClient.Build(buildkitCtx, client.SolveOpt{
	// 	AllowedEntitlements: []entitlements.Entitlement{
	// 		entitlements.EntitlementNetworkHost, // allow --network=host
	// 	},
	// 	Internal: true,
	// 	// CacheExports: []client.CacheOptionsEntry{
	// 	// 	{
	// 	// 		Type: client.ExporterOCI,
	// 	// 		Attrs: map[string]string{
	// 	// 			"dest": filepath.Join("DinD", "cache"),
	// 	// 		},
	// 	// 	},
	// 	// },
	// 	// CacheImports: []client.CacheOptionsEntry{
	// 	// 	{
	// 	// 		Type: client.ExporterOCI,
	// 	// 		Attrs: map[string]string{
	// 	// 			"src": filepath.Join("DinD", "cache"),
	// 	// 		},
	// 	// 	},
	// 	// },
	// }, "packerfile.v0", bkBldr.BuildkitBuilderBuild, nil)
	// if err != nil {
	// 	return err
	// }

	rawBuilderImage, err := c.imageFetcher.Fetch(ctx, builderRef.Name(), image.FetchOptions{Daemon: true, PullPolicy: opts.PullPolicy})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch builder image '%s'", builderRef.Name())
	}

	builderOS, err := rawBuilderImage.OS()
	if err != nil {
		return errors.Wrapf(err, "getting builder OS")
	}

	builderArch, err := rawBuilderImage.Architecture()
	if err != nil {
		return errors.Wrapf(err, "getting builder architecture")
	}

	bldr, err := c.getBuilder(rawBuilderImage)
	if err != nil {
		return errors.Wrapf(err, "invalid builder %s", style.Symbol(opts.Builder))
	}

	runImageName := c.resolveRunImage(opts.RunImage, imgRegistry, builderRef.Context().RegistryStr(), bldr.DefaultRunImage(), opts.AdditionalMirrors, opts.Publish, c.accessChecker)

	fetchOptions := image.FetchOptions{
		Daemon:     !opts.Publish,
		PullPolicy: opts.PullPolicy,
		Platform:   fmt.Sprintf("%s/%s", builderOS, builderArch),
	}
	if opts.Layout() {
		targetRunImagePath, err := layout.ParseRefToPath(runImageName)
		if err != nil {
			return err
		}
		hostRunImagePath := filepath.Join(opts.LayoutConfig.LayoutRepoDir, targetRunImagePath)
		targetRunImagePath = filepath.Join(paths.RootDir, "layout-repo", targetRunImagePath)
		fetchOptions.LayoutOption = image.LayoutOption{
			Path:   hostRunImagePath,
			Sparse: opts.LayoutConfig.Sparse,
		}
		fetchOptions.Daemon = false
		pathsConfig.targetRunImagePath = targetRunImagePath
		pathsConfig.hostRunImagePath = hostRunImagePath
	}
	runImage, err := c.validateRunImage(ctx, runImageName, fetchOptions, bldr.StackID)
	if err != nil {
		return errors.Wrapf(err, "invalid run-image '%s'", runImageName)
	}

	var runMixins []string
	if _, err := dist.GetLabel(runImage, stack.MixinsLabel, &runMixins); err != nil {
		return err
	}

	fetchedBPs, order, err := c.processBuildpacks(ctx, bldr.Image(), bldr.Buildpacks(), bldr.Order(), bldr.StackID, opts)
	if err != nil {
		return err
	}

	fetchedExs, orderExtensions, err := c.processExtensions(ctx, bldr.Image(), bldr.Extensions(), bldr.OrderExtensions(), bldr.StackID, opts)
	if err != nil {
		return err
	}

	// Default mode: if the TrustBuilder option is not set, trust the suggested builders.
	if opts.TrustBuilder == nil {
		opts.TrustBuilder = IsTrustedBuilderFunc
	}

	// Ensure the builder's platform APIs are supported
	var builderPlatformAPIs builder.APISet
	builderPlatformAPIs = append(builderPlatformAPIs, bldr.LifecycleDescriptor().APIs.Platform.Deprecated...)
	builderPlatformAPIs = append(builderPlatformAPIs, bldr.LifecycleDescriptor().APIs.Platform.Supported...)
	if !supportsPlatformAPI(builderPlatformAPIs) {
		c.logger.Debugf("pack %s supports Platform API(s): %s", c.version, strings.Join(build.SupportedPlatformAPIVersions.AsStrings(), ", "))
		c.logger.Debugf("Builder %s supports Platform API(s): %s", style.Symbol(opts.Builder), strings.Join(builderPlatformAPIs.AsStrings(), ", "))
		return errors.Errorf("Builder %s is incompatible with this version of pack", style.Symbol(opts.Builder))
	}

	// Get the platform API version to use
	lifecycleVersion := bldr.LifecycleDescriptor().Info.Version
	useCreator := supportsCreator(lifecycleVersion) && opts.TrustBuilder(opts.Builder)
	var (
		lifecycleOptsLifecycleImage string
		lifecycleAPIs               []string
	)
	if !(useCreator) {
		// fetch the lifecycle image
		if supportsLifecycleImage(lifecycleVersion) {
			lifecycleImageName := opts.LifecycleImage
			if lifecycleImageName == "" {
				lifecycleImageName = fmt.Sprintf("%s:%s", internalConfig.DefaultLifecycleImageRepo, lifecycleVersion.String())
			}

			lifecycleImage, err := c.imageFetcher.Fetch(
				ctx,
				lifecycleImageName,
				image.FetchOptions{
					Daemon:     true,
					PullPolicy: opts.PullPolicy,
					Platform:   fmt.Sprintf("%s/%s", builderOS, builderArch),
				},
			)
			if err != nil {
				return fmt.Errorf("fetching lifecycle image: %w", err)
			}

			lifecycleOptsLifecycleImage = lifecycleImage.Name()
			labels, err := lifecycleImage.Labels()
			if err != nil {
				return fmt.Errorf("reading labels of lifecycle image: %w", err)
			}

			lifecycleAPIs, err = extractSupportedLifecycleApis(labels)
			if err != nil {
				return fmt.Errorf("reading api versions of lifecycle image: %w", err)
			}
		}
	}

	usingPlatformAPI, err := build.FindLatestSupported(append(
		bldr.LifecycleDescriptor().APIs.Platform.Deprecated,
		bldr.LifecycleDescriptor().APIs.Platform.Supported...),
		lifecycleAPIs)
	if err != nil {
		return fmt.Errorf("finding latest supported Platform API: %w", err)
	}
	if usingPlatformAPI.LessThan("0.12") {
		if err = c.validateMixins(fetchedBPs, bldr, runImageName, runMixins); err != nil {
			return fmt.Errorf("validating stack mixins: %w", err)
		}
	}

	buildEnvs := map[string]string{}
	for _, envVar := range opts.ProjectDescriptor.Build.Env {
		buildEnvs[envVar.Name] = envVar.Value
	}

	for k, v := range opts.Env {
		buildEnvs[k] = v
	}

	ephemeralBuilder, err := c.createEphemeralBuilder(rawBuilderImage, buildEnvs, order, fetchedBPs, orderExtensions, fetchedExs, usingPlatformAPI.LessThan("0.12"), opts.RunImage)
	if err != nil {
		return err
	}
	defer c.docker.ImageRemove(context.Background(), ephemeralBuilder.Name(), types.ImageRemoveOptions{Force: true})

	if len(bldr.OrderExtensions()) > 0 || len(ephemeralBuilder.OrderExtensions()) > 0 {
		if !c.experimental {
			return fmt.Errorf("experimental features must be enabled when builder contains image extensions")
		}
		if builderOS == "windows" {
			return fmt.Errorf("builder contains image extensions which are not supported for Windows builds")
		}
		if !(opts.PullPolicy == image.PullAlways) {
			return fmt.Errorf("pull policy must be 'always' when builder contains image extensions")
		}
	}

	if opts.Layout() {
		opts.ContainerConfig.Volumes = appendLayoutVolumes(opts.ContainerConfig.Volumes, pathsConfig)
	}

	// now bui
	processedVolumes, warnings, err := processVolumes(builderOS, opts.ContainerConfig.Volumes)
	if err != nil {
		return err
	}

	for _, warning := range warnings {
		c.logger.Warn(warning)
	}

	fileFilter, err := getFileFilter(opts.ProjectDescriptor)
	if err != nil {
		return err
	}

	runImageName, err = pname.TranslateRegistry(runImageName, c.registryMirrors, c.logger)
	if err != nil {
		return err
	}

	projectMetadata := files.ProjectMetadata{}
	if c.experimental {
		version := opts.ProjectDescriptor.Project.Version
		sourceURL := opts.ProjectDescriptor.Project.SourceURL
		if version != "" || sourceURL != "" {
			projectMetadata.Source = &files.ProjectSource{
				Type:     "project",
				Version:  map[string]interface{}{"declared": version},
				Metadata: map[string]interface{}{"url": sourceURL},
			}
		} else {
			projectMetadata.Source = v02.GitMetadata(opts.AppPath)
		}
	}

	lifecycleOpts := build.LifecycleOptions{
		AppPath:                  appPath,
		Image:                    imageRef,
		Builder:                  ephemeralBuilder,
		BuilderImage:             builderRef.Name(),
		LifecycleImage:           ephemeralBuilder.Name(),
		RunImage:                 runImageName,
		ProjectMetadata:          projectMetadata,
		ClearCache:               opts.ClearCache,
		Publish:                  opts.Publish,
		TrustBuilder:             opts.TrustBuilder(opts.Builder),
		UseCreator:               useCreator,
		UseCreatorWithExtensions: supportsCreatorWithExtensions(lifecycleVersion),
		DockerHost:               opts.DockerHost,
		Cache:                    opts.Cache,
		CacheImage:               opts.CacheImage,
		HTTPProxy:                proxyConfig.HTTPProxy,
		HTTPSProxy:               proxyConfig.HTTPSProxy,
		NoProxy:                  proxyConfig.NoProxy,
		Network:                  opts.ContainerConfig.Network,
		AdditionalTags:           opts.AdditionalTags,
		Volumes:                  processedVolumes,
		DefaultProcessType:       opts.DefaultProcessType,
		FileFilter:               fileFilter,
		Workspace:                opts.Workspace,
		GID:                      opts.GroupID,
		UID:                      opts.UserID,
		PreviousImage:            opts.PreviousImage,
		Interactive:              opts.Interactive,
		Termui:                   termui.NewTermui(imageName, ephemeralBuilder, runImageName),
		ReportDestinationDir:     opts.ReportDestinationDir,
		SBOMDestinationDir:       opts.SBOMDestinationDir,
		CreationTime:             opts.CreationTime,
		Layout:                   opts.Layout(),
		Keychain:                 c.keychain,
	}

	switch {
	case useCreator:
		lifecycleOpts.UseCreator = true
	case supportsLifecycleImage(lifecycleVersion):
		lifecycleOpts.LifecycleImage = lifecycleOptsLifecycleImage
		lifecycleOpts.LifecycleApis = lifecycleAPIs
	case !opts.TrustBuilder(opts.Builder):
		return errors.Errorf("Lifecycle %s does not have an associated lifecycle image. Builder must be trusted.", lifecycleVersion.String())
	}

	lifecycleOpts.FetchRunImageWithLifecycleLayer = func(runImageName string) (string, error) {
	ephemeralRunImageName := fmt.Sprintf("pack.local/run-image/%x:latest", randString(10))
	runImage, err := c.imageFetcher.Fetch(ctx, runImageName, fetchOptions)
	if err != nil {
		return "", err
	}
	ephemeralRunImage, err := local.NewImage(ephemeralRunImageName, c.docker, local.FromBaseImage(runImage.Name()))
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp("", "extend-run-image-buidlerState") // we need to write to disk because manifest.json is last in the tar
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	lifecycleImageTar, err := func() (string, error) {
		lifecycleImageTar := filepath.Join(tmpDir, "lifecycle-image.tar")
		lifecycleImageReader, err := c.docker.ImageSave(context.Background(), []string{lifecycleOpts.LifecycleImage}) // this is fast because the lifecycle image is based on distroless static
		if err != nil {
			return "", err
		}
		defer lifecycleImageReader.Close()
		lifecycleImageWriter, err := os.Create(lifecycleImageTar)
		if err != nil {
			return "", err
		}
		defer lifecycleImageWriter.Close()
		if _, err = io.Copy(lifecycleImageWriter, lifecycleImageReader); err != nil {
			return "", err
		}
		return lifecycleImageTar, nil
	}()
	if err != nil {
		return "", err
	}
	advanceTarToEntryWithName := func(tarReader *tar.Reader, wantName string) (*tar.Header, error) {
		var (
			header *tar.Header
			err    error
		)
		for {
			header, err = tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if header.Name != wantName {
				continue
			}
			return header, nil
		}
		return nil, fmt.Errorf("failed to find header with name: %s", wantName)
	}
	lifecycleLayerName, err := func() (string, error) {
		lifecycleImageReader, err := os.Open(lifecycleImageTar)
		if err != nil {
			return "", err
		}
		defer lifecycleImageReader.Close()
		tarReader := tar.NewReader(lifecycleImageReader)
		if _, err = advanceTarToEntryWithName(tarReader, "manifest.json"); err != nil {
			return "", err
		}
		type descriptor struct {
			Layers []string
		}
		type manifestJSON []descriptor
		var manifestContents manifestJSON
		if err = json.NewDecoder(tarReader).Decode(&manifestContents); err != nil {
			return "", err
		}
		if len(manifestContents) < 1 {
			return "", errors.New("missing manifest entries")
		}
		return manifestContents[0].Layers[len(manifestContents[0].Layers)-1], nil // we can assume the lifecycle layer is the last in the tar
	}()
	if err != nil {
		return "", err
	}
	if lifecycleLayerName == "" {
		return "", errors.New("failed to find lifecycle layer")
	}
	lifecycleLayerTar, err := func() (string, error) {
		lifecycleImageReader, err := os.Open(lifecycleImageTar)
		if err != nil {
			return "", err
		}
		defer lifecycleImageReader.Close()
		tarReader := tar.NewReader(lifecycleImageReader)
		var header *tar.Header
		if header, err = advanceTarToEntryWithName(tarReader, lifecycleLayerName); err != nil {
			return "", err
		}
		lifecycleLayerTar := filepath.Join(filepath.Dir(lifecycleImageTar), filepath.Dir(lifecycleLayerName)+".tar")
		lifecycleLayerWriter, err := os.OpenFile(lifecycleLayerTar, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
		if err != nil {
			return "", err
		}
		defer lifecycleLayerWriter.Close()
		if _, err = io.Copy(lifecycleLayerWriter, tarReader); err != nil {
			return "", err
		}
		return lifecycleLayerTar, nil
	}()
	if err != nil {
		return "", err
	}
	diffID, err := func() (string, error) {
		lifecycleLayerReader, err := os.Open(lifecycleLayerTar)
		if err != nil {
			return "", err
		}
		defer lifecycleLayerReader.Close()
		hasher := sha256.New()
		if _, err = io.Copy(hasher, lifecycleLayerReader); err != nil {
			return "", err
		}
		// it's weird that this doesn't match lifecycleLayerTar
		return hex.EncodeToString(hasher.Sum(nil)), nil
	}()
	if err != nil {
		return "", err
	}
	if err = ephemeralRunImage.AddLayerWithDiffID(lifecycleLayerTar, "sha256:"+diffID); err != nil {
		return "", err
	}
	if err = ephemeralRunImage.Save(); err != nil {
		return "", err
	}
	return ephemeralRunImageName, nil
}

	// implement with buildkit
	c.lifecycleExecutor = executor.New(c.docker, *buidlerState, c.logger, opts.Targets)
	if err = c.lifecycleExecutor.Execute(ctx, lifecycleOpts); err != nil {
		return fmt.Errorf("executing lifecycle: %w", err)
	}
	return c.logImageNameAndSha(ctx, opts.Publish, imageRef)
}

// type Builder struct {
// 	appName name.Reference
// 	ref name.Reference
// 	*state.State
// 	prevImage *state.State
// 	platforms []ocispecs.Platform
// 	client *Client
// 	opts BuildOptions
// }

// func NewBuilder(appName, bldrName string, client *Client, platforms []ocispecs.Platform, opts BuildOptions) (*Builder, error) {
// 	bldrRef, err := name.ParseReference(bldrName)
// 	if err != nil {
// 		return nil, err
// 	}

// 	appRef, err := name.ParseReference(appName)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return &Builder{
// 		platforms: platforms,
// 		ref: bldrRef,
// 		appName: appRef,
// 		client: client,
// 		opts: opts,
// 	}, nil
// }

// func (b *Builder) BuildkitBuilderBuild(ctx context.Context, c gatewayClient.Client) (res *gatewayClient.Result, err error) {
// 	res = gatewayClient.NewResult()
// 	expPlatforms := &exptypes.Platforms{
// 		Platforms: make([]exptypes.Platform, len(b.platforms)),
// 	}

// 	res.AddMeta("image.name", []byte(b.ref.Name())) // added an annotation to the image/index manifest
// 	eg, ctx1 := errgroup.WithContext(ctx)
// 	for i, platform := range b.platforms {
// 		i, platform := i, platform
// 		eg.Go(func() error {
// 			def, err := b.State.State().Marshal(ctx1, llb.Platform(platform))
// 			if err != nil {
// 				return errors.Wrap(err, "failed to marshal state")
// 			}

// 			r, err := c.Solve(ctx1, gatewayClient.SolveRequest{
// 				// CacheImports: b.cacheImports, // TODO: update cache imports to [pack home]
// 				Definition:   def.ToPB(),
// 			})
// 			if err != nil {
// 				return errors.Wrap(err, "failed to solve")
// 			}

// 			ref, err := r.SingleRef()
// 			if err != nil {
// 				return err
// 			}

// 			// _, err = ref.ToState()
// 			// if err != nil {
// 			// 	return err
// 			// }

// 			p := platforms.Format(platform)
// 			res.AddRef(p, ref)
// 			fmt.Printf("\n formatted platform: %s\n", p)

// 			config := b.State.ConfigFile()
// 			mutateConfigFile(config, platform)
// 			configBytes, err := json.Marshal(config)
// 			if err != nil {
// 				return err
// 			}

// 			res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageConfigKey, p), configBytes)
// 			if b.prevImage != nil {
// 				baseConfig := b.prevImage.ConfigFile()
// 				mutateConfigFile(baseConfig, platform)
// 				configBytes, err := json.Marshal(baseConfig)
// 				if err != nil {
// 					return err
// 				}
// 				res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageBaseConfigKey, p), configBytes)
// 			}

// 			expPlatforms.Platforms[i] = exptypes.Platform{
// 				ID:       p,
// 				Platform: platform,
// 			}
// 			fmt.Printf("\n export platform at %d is %s/%s/%s\n", i, platform.OS, platform.Architecture, platform.Variant)

// 			// var mfest *ocispecs.Image
// 			// mfestBytes := res.Metadata[exptypes.ExporterImageConfigKey]
// 			// if err := json.Unmarshal(mfestBytes, mfest); err != nil {
// 			// 	return err
// 			// }

// 			// bkBldr, err := builder.NewBuildkitBuilder(res, b.ref.Name(), platform)
// 			// if err != nil {
// 			// 	return errors.Wrapf(err, "invalid builder %s(%s)", style.Symbol(b.ref.Name()), p)
// 			// }

// 			// runImageName := b.client.resolveRunImage(b.opts.RunImage, b.appName.Context().RegistryStr(), b.ref.Context().RegistryStr(), bkBldr.DefaultRunImage(), b.opts.AdditionalMirrors, b.opts.Publish, b.client.accessChecker)
// 			// runImgRes, err := b.validateBuildkitRunImage(ctx, runImageName, platform, bkBldr.StackID)
// 			// if err != nil {
// 			// 	return errors.Wrapf(err, "invalid run-image '%s'", runImageName)
// 			// }

// 			// var runMixins []string
// 			// if _, err := dist.GetBuildkitLabel(runImgRes, stack.MixinsLabel, &runMixins); err != nil {
// 			// 	return err
// 			// }
// 			return nil
// 		})
// 	}

// 	if err := eg.Wait(); err != nil {
// 		return nil, err
// 	}

// 	dt, err := json.Marshal(expPlatforms)
// 	if err != nil {
// 		return res, errors.Wrap(err, "failed to marshal the target platforms")
// 	}

// 	fmt.Printf("\n multi-arch export platform: %v", expPlatforms.Platforms)

// 	res.AddMeta(exptypes.ExporterPlatformsKey, dt)
// 	return res, err
// }

// func (b *Builder) validateBuildkitRunImage(ctx context.Context, name string, platform ocispecs.Platform, expectedStack string) (res *gatewayClient.Result, err error) {
// 	if name == "" {
// 		return nil, errors.New("run image must be specified")
// 	}

// 	var runImageState *state.State
// 	switch b.opts.PullPolicy {
// 	case image.PullNever:
// 		runImageState = state.Local(name, llb.Platform(platform))
// 	case image.PullAlways:
// 		runImageState = state.Remote(name, llb.MarkImageInternal, llb.Platform(platform))
// 	default:
// 		runImageState = state.Local(name, llb.Platform(platform))
// 		// if err := state.Validate(ctx, llb.NewConstraints(llb.LocalUniqueID(identity.NewID()))); err != nil {
// 		// 	// lets not validate llb.Image
// 		// 	state = llb.Image(builderRef.Name())
// 		// }
// 	}

// 	def, err := runImageState.State().Marshal(ctx)
// 	if err != nil {
// 		return res, err
// 	}

// 	err = grpcclient.RunFromEnvironment(ctx, func(ctx context.Context, c gatewayClient.Client) (*gatewayClient.Result, error) {
// 		return c.Solve(ctx, gatewayClient.SolveRequest{
// 			Definition: def.ToPB(),
// 			// CacheImports: []gatewayClient.CacheOptionsEntry{
// 			// 	{
// 			// 		Type: client.ExporterOCI,
// 			// 		Attrs: map[string]string{
// 			// 			"src": filepath.Join("DinD", "cache"),
// 			// 		},
// 			// 	},
// 			// },
// 		})
// 	})
// 	if err != nil {
// 		return res, err
// 	}

// 	platformStr := platforms.Format(platform)
// 	bkBldr, err := builder.NewBuildkitBuilder(res, name, platform)
// 	if err != nil {
// 		return res, errors.Wrapf(err, "invalid runImage %s(%s)", style.Symbol(b.ref.Name()), platformStr)
// 	}

// 	stackID, err := bkBldr.Label("io.buildpacks.stack.id")
// 	if err != nil {
// 		return res, errors.Wrap(err, "resolving runImage stackID")
// 	}

// 	if stackID != expectedStack {
// 		return nil, fmt.Errorf("run-image stack id '%s' does not match builder stack '%s'", stackID, expectedStack)
// 	}
// 	return res, err
// }

// func mutateConfigFile(config *v1.ConfigFile, platform ocispecs.Platform) {
// 	if config == nil {
// 		config = &v1.ConfigFile{}
// 	}
// 	config.OS = platform.OS
// 	config.Architecture = platform.Architecture
// 	config.Variant = platform.Variant
// 	config.OSVersion = platform.OSVersion
// 	config.OSFeatures = platform.OSFeatures
// }
