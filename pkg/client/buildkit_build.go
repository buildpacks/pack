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
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	state "github.com/buildpacks/pack/internal/buildkit/build_state"
	"github.com/buildpacks/pack/internal/buildkit/executor"
	internalConfig "github.com/buildpacks/pack/internal/config"
	pname "github.com/buildpacks/pack/internal/name"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/termui"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	v02 "github.com/buildpacks/pack/pkg/project/v02"
	"github.com/docker/docker/api/types"
	"github.com/moby/buildkit/client/llb"
	"github.com/pkg/errors"
)

// BuildWithBuildkit creates an ephemeral builder from the user provided builder.
// It adds the additional buildpacks and lifecycles if needed
// It is powered with buildkit and uses advanced caching to speed up builds
// when recreating the ephemeral builder
//
// BuildWithBuildkit configures settings for the build container(s) and lifecycle.
// It then invokes the lifecycle to build an app image.
// If any configuration is deemed invalid, or if any lifecycle phases fail,
// an error will be returned and no image produced.
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

	var buidlerState *state.State
	switch opts.PullPolicy {
	case image.PullAlways:
		buidlerState = state.Remote(builderRef.Name(), llb.WithCustomName("pulling builder image..."), llb.ResolveModePreferLocal).Network(llb.NetModeHost.String()) // llb.ResolveModeForcePull
	case image.PullNever:
		buidlerState = state.Remote(builderRef.Name(), llb.WithCustomName("pulling builder image..."), llb.ResolveModePreferLocal).Network(llb.NetModeHost.String())
	default:
		buidlerState = state.Remote(builderRef.Name(), llb.WithCustomName("pulling builder image..."), llb.ResolveModePreferLocal).Network(llb.NetModeHost.String()) // llb.ResolveModeDefault
	}


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

	fetchOptions := image.FetchOptions{
		Daemon:     !opts.Publish,
		PullPolicy: opts.PullPolicy,
		Platform:   fmt.Sprintf("%s/%s", builderOS, builderArch),
	}
	runImageName := c.resolveRunImage(opts.RunImage, imgRegistry, builderRef.Context().RegistryStr(), bldr.DefaultRunImage(), opts.AdditionalMirrors, opts.Publish, fetchOptions)

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

			// if lifecyle container os isn't windows, use ephemeral lifecycle to add /workspace with correct ownership
			imageOS, err := lifecycleImage.OS()
			if err != nil {
				return errors.Wrap(err, "getting lifecycle image OS")
			}
			if imageOS != "windows" {
				// obtain uid/gid from builder to use when extending lifecycle image
				uid, gid, err := userAndGroupIDs(rawBuilderImage)
				if err != nil {
					return fmt.Errorf("obtaining build uid/gid from builder image: %w", err)
				}

				c.logger.Debugf("Creating ephemeral lifecycle from %s with uid %d and gid %d. With workspace dir %s", lifecycleImage.Name(), uid, gid, opts.Workspace)
				// extend lifecycle image with mountpoints, and use it instead of current lifecycle image
				lifecycleImage, err = c.createEphemeralLifecycle(lifecycleImage, opts.Workspace, uid, gid)
				if err != nil {
					return err
				}
				c.logger.Debugf("Selecting ephemeral lifecycle image %s for build", lifecycleImage.Name())
				// cleanup the extended lifecycle image when done
				defer c.docker.ImageRemove(context.Background(), lifecycleImage.Name(), types.ImageRemoveOptions{Force: true})
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
		tmpDir, err := os.MkdirTemp("", "extend-run-image-scratch") // we need to write to disk because manifest.json is last in the tar
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
			// we can assume the lifecycle layer is the last in the tar, except if the lifecycle has been extended as an ephemeral lifecycle
			layerOffset := 1
			if strings.Contains(lifecycleOpts.LifecycleImage, "pack.local/lifecycle") {
				layerOffset = 2
			}

			if (len(manifestContents[0].Layers) - layerOffset) < 0 {
				return "", errors.New("Lifecycle image did not contain expected layer count")
			}

			return manifestContents[0].Layers[len(manifestContents[0].Layers)-layerOffset], nil
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

	// switch opts.PullPolicy {
	// case image.PullAlways:
	// 	buidlerState = state.Remote(ephemeralBuilder.Name(), llb.WithCustomName("pulling ephermeral builder image..."), llb.ResolveModePreferLocal).Network(llb.NetModeHost.String()) // llb.ResolveModeForcePull
	// case image.PullNever:
	// 	buidlerState = state.Remote(ephemeralBuilder.Name(), llb.WithCustomName("pulling ephermeral builder image..."), llb.ResolveModePreferLocal).Network(llb.NetModeHost.String())
	// default:
	// 	buidlerState = state.Remote(ephemeralBuilder.Name(), llb.WithCustomName("pulling ephermeral builder image..."), llb.ResolveModePreferLocal).Network(llb.NetModeHost.String()) // llb.ResolveModeDefault
	// }

	// the Client#lifecycleExecutor defaults to docker's lifecycle executor
	// replace this executor with the buildkit one to build with buildkit
	c.lifecycleExecutor, err = executor.New(ctx, c.docker, buidlerState, c.logger, []dist.Target{}) // TODO: replace []dist.Target{} with targets from cli by creating a new field in BuildOptions
	if err != nil {
		return err
	}
	
	if err = c.lifecycleExecutor.Execute(ctx, lifecycleOpts); err != nil {
		return fmt.Errorf("executing lifecycle with buildkit: %w", err)
	}
	return c.logImageNameAndSha(ctx, opts.Publish, imageRef)
}
