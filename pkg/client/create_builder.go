package client

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/image"
)

// CreateBuilderOptions is a configuration object used to change the behavior of
// CreateBuilder.
type CreateBuilderOptions struct {
	// The base directory to use to resolve relative assets
	RelativeBaseDir string

	// Name of the builder.
	BuilderName string

	// BuildConfigEnv for Builder
	BuildConfigEnv map[string]string

	// Map of labels to add to the Buildpack
	Labels map[string]string

	// Configuration that defines the functionality a builder provides.
	Config pubbldr.MultiArchConfig

	// Skip building image locally, directly publish to a registry.
	// Requires BuilderName to be a valid registry location.
	Publish bool

	// Buildpack registry name. Defines where all registry buildpacks will be pulled from.
	Registry string

	// Strategy for updating images before a build.
	PullPolicy image.PullPolicy

	// List of modules to be flattened
	Flatten buildpack.FlattenModuleInfos

	// Optional: ImageIndex used for creating IndexManifest
	ImageIndex imgutil.ImageIndex
}

// CreateBuilder creates and saves a builder image to a registry with the provided options.
// If any configuration is invalid, it will error and exit without creating any images.
func (c *Client) CreateBuilder(ctx context.Context, opts CreateBuilderOptions) error {
	if err := c.validateConfig(ctx, opts); err != nil {
		return err
	}

	bldr, err := c.createBaseBuilder(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "failed to create builder")
	}

	if err := c.addBuildpacksToBuilder(ctx, opts, bldr); err != nil {
		return errors.Wrap(err, "failed to add buildpacks to builder")
	}

	if err := c.addExtensionsToBuilder(ctx, opts, bldr); err != nil {
		return errors.Wrap(err, "failed to add extensions to builder")
	}

	bldr.SetOrder(opts.Config.Order)
	bldr.SetOrderExtensions(opts.Config.OrderExtensions)

	if opts.Config.Stack.ID != "" {
		bldr.SetStack(opts.Config.Stack)
	}
	bldr.SetRunImage(opts.Config.Run)
	bldr.SetBuildConfigEnv(opts.BuildConfigEnv)

	if err := bldr.Save(c.logger, builder.CreatorMetadata{Version: c.version}); err != nil {
		return err
	}

	if opts.ImageIndex == nil {
		return nil
	}

	ref, err := name.ParseReference(opts.BuilderName, name.Insecure, name.WeakValidation)
	if err != nil {
		return err
	}

	underlyingImage := bldr.Image().UnderlyingImage()
	if underlyingImage != nil {
		digest, err := underlyingImage.Digest()
		if err != nil {
			return err
		}

		if err := opts.ImageIndex.Add(ref.Context().Digest(digest.String()), imgutil.WithLocalImage(bldr.Image())); err != nil {
			return err
		}
	} else {
		id, err := bldr.Image().Identifier()
		if err != nil {
			return err
		}

		digest := ref.Context().Digest("sha256:" + id.String())
		if err := opts.ImageIndex.Add(ref.Context().Digest(digest.String()), imgutil.WithLocalImage(bldr.Image())); err != nil {
			return err
		}
	}

	return opts.ImageIndex.Save()
}

func (c *Client) CreateMultiArchBuilder(ctx context.Context, opts CreateBuilderOptions) error {
	if !c.experimental {
		return errors.Errorf("creating %s is currently %s", style.Symbol("multi arch builder"), style.Symbol(("experimental")))
	}

	if err := c.validateConfig(ctx, opts); err != nil {
		return err
	}

	configs, err := opts.Config.BuilderConfigs(c.GetIndexManifestFn())
	if err != nil {
		return err
	}

	if err := createImageIndex(c, opts.BuilderName); err != nil {
		return err
	}

	idx, err := loadImageIndex(c, opts.BuilderName)
	if err != nil {
		return err
	}

	// var errs errgroup.Group
	ops := opts
	ops.ImageIndex = idx
	for _, config := range configs {
		ops.Config.Config = config
		// errs.Go(func() error {
		if err := c.CreateBuilder(ctx, ops); err != nil {
			return err
		}
		// })
	}

	// if err := errs.Wait(); err != nil {
	// 	return err
	// }

	if !opts.Publish {
		return nil
	}

	if err := idx.Save(); err != nil {
		return err
	}

	return idx.Push(imgutil.WithInsecure(true), imgutil.WithTags("latest"))
}

func (c *Client) validateConfig(ctx context.Context, opts CreateBuilderOptions) error {
	if err := pubbldr.ValidateConfig(opts.Config.Config); err != nil {
		return errors.Wrap(err, "invalid builder config")
	}

	if err := c.validateRunImageConfig(ctx, opts); err != nil {
		return errors.Wrap(err, "invalid run image config")
	}

	return nil
}

func (c *Client) validateRunImageConfig(ctx context.Context, opts CreateBuilderOptions) error {
	var runImages []imgutil.Image
	for _, r := range opts.Config.Run.Images {
		for _, i := range append([]string{r.Image}, r.Mirrors...) {
			if !opts.Publish {
				img, err := c.imageFetcher.Fetch(ctx, i, image.FetchOptions{Daemon: true, PullPolicy: opts.PullPolicy})
				if err != nil {
					if errors.Cause(err) != image.ErrNotFound {
						return errors.Wrap(err, "failed to fetch image")
					}
				} else {
					runImages = append(runImages, img)
					continue
				}
			}

			img, err := c.imageFetcher.Fetch(ctx, i, image.FetchOptions{Daemon: false, PullPolicy: opts.PullPolicy})
			if err != nil {
				if errors.Cause(err) != image.ErrNotFound {
					return errors.Wrap(err, "failed to fetch image")
				}
				c.logger.Warnf("run image %s is not accessible", style.Symbol(i))
			} else {
				runImages = append(runImages, img)
			}
		}
	}

	for _, img := range runImages {
		if opts.Config.Stack.ID != "" {
			stackID, err := img.Label("io.buildpacks.stack.id")
			if err != nil {
				return errors.Wrap(err, "failed to label image")
			}

			if stackID != opts.Config.Stack.ID {
				return fmt.Errorf(
					"stack %s from builder config is incompatible with stack %s from run image %s",
					style.Symbol(opts.Config.Stack.ID),
					style.Symbol(stackID),
					style.Symbol(img.Name()),
				)
			}
		}
	}

	return nil
}

func (c *Client) createBaseBuilder(ctx context.Context, opts CreateBuilderOptions) (*builder.Builder, error) {
	baseImage, err := c.imageFetcher.Fetch(ctx, opts.Config.Build.Image, image.FetchOptions{Daemon: !opts.Publish, PullPolicy: opts.PullPolicy})
	if err != nil {
		return nil, errors.Wrap(err, "fetch build image")
	}

	c.logger.Debugf("Creating builder %s from build-image %s", style.Symbol(opts.BuilderName), style.Symbol(baseImage.Name()))

	var builderOpts []builder.BuilderOption
	if opts.Flatten != nil && len(opts.Flatten.FlattenModules()) > 0 {
		builderOpts = append(builderOpts, builder.WithFlattened(opts.Flatten))
	}
	if opts.Labels != nil && len(opts.Labels) > 0 {
		builderOpts = append(builderOpts, builder.WithLabels(opts.Labels))
	}

	bldr, err := builder.New(baseImage, opts.BuilderName, builderOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "invalid build-image")
	}

	architecture, err := baseImage.Architecture()
	if err != nil {
		return nil, errors.Wrap(err, "lookup image Architecture")
	}

	os, err := baseImage.OS()
	if err != nil {
		return nil, errors.Wrap(err, "lookup image OS")
	}

	if os == "windows" && !c.experimental {
		return nil, NewExperimentError("Windows containers support is currently experimental.")
	}

	bldr.SetDescription(opts.Config.Description)

	if opts.Config.Stack.ID != "" && bldr.StackID != opts.Config.Stack.ID {
		return nil, fmt.Errorf(
			"stack %s from builder config is incompatible with stack %s from build image",
			style.Symbol(opts.Config.Stack.ID),
			style.Symbol(bldr.StackID),
		)
	}

	lifecycle, err := c.fetchLifecycle(ctx, opts.Config.Lifecycle, opts.RelativeBaseDir, os, architecture)
	if err != nil {
		return nil, errors.Wrap(err, "fetch lifecycle")
	}

	bldr.SetLifecycle(lifecycle)
	bldr.SetBuildConfigEnv(opts.BuildConfigEnv)

	return bldr, nil
}

func (c *Client) fetchLifecycle(ctx context.Context, config pubbldr.LifecycleConfig, relativeBaseDir, os string, architecture string) (builder.Lifecycle, error) {
	if config.Version != "" && config.URI != "" {
		return nil, errors.Errorf(
			"%s can only declare %s or %s, not both",
			style.Symbol("lifecycle"), style.Symbol("version"), style.Symbol("uri"),
		)
	}

	var uri string
	var err error
	switch {
	case config.Version != "":
		v, err := semver.NewVersion(config.Version)
		if err != nil {
			return nil, errors.Wrapf(err, "%s must be a valid semver", style.Symbol("lifecycle.version"))
		}

		uri = uriFromLifecycleVersion(*v, os, architecture)
	case config.URI != "":
		uri, err = paths.FilePathToURI(config.URI, relativeBaseDir)
		if err != nil {
			return nil, err
		}
	default:
		uri = uriFromLifecycleVersion(*semver.MustParse(builder.DefaultLifecycleVersion), os, architecture)
	}

	blob, err := c.downloader.Download(ctx, uri)
	if err != nil {
		return nil, errors.Wrap(err, "downloading lifecycle")
	}

	lifecycle, err := builder.NewLifecycle(blob)
	if err != nil {
		return nil, errors.Wrap(err, "invalid lifecycle")
	}

	return lifecycle, nil
}

func (c *Client) addBuildpacksToBuilder(ctx context.Context, opts CreateBuilderOptions, bldr *builder.Builder) error {
	for _, b := range opts.Config.Buildpacks {
		if err := c.addConfig(ctx, buildpack.KindBuildpack, b, opts, bldr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) addExtensionsToBuilder(ctx context.Context, opts CreateBuilderOptions, bldr *builder.Builder) error {
	for _, e := range opts.Config.Extensions {
		if err := c.addConfig(ctx, buildpack.KindExtension, e, opts, bldr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) addConfig(ctx context.Context, kind string, config pubbldr.ModuleConfig, opts CreateBuilderOptions, bldr *builder.Builder) error {
	c.logger.Debugf("Looking up %s %s", kind, style.Symbol(config.DisplayString()))

	builderOS, err := bldr.Image().OS()
	if err != nil {
		return errors.Wrapf(err, "getting builder OS")
	}
	builderArch, err := bldr.Image().Architecture()
	if err != nil {
		return errors.Wrapf(err, "getting builder architecture")
	}

	mainBP, depBPs, err := c.buildpackDownloader.Download(ctx, config.URI, buildpack.DownloadOptions{
		Daemon:          !opts.Publish,
		ImageName:       config.ImageName,
		ImageOS:         builderOS,
		Platform:        fmt.Sprintf("%s/%s", builderOS, builderArch),
		ModuleKind:      kind,
		PullPolicy:      opts.PullPolicy,
		RegistryName:    opts.Registry,
		RelativeBaseDir: opts.RelativeBaseDir,
	})
	if err != nil {
		return errors.Wrapf(err, "downloading %s", kind)
	}
	err = validateModule(kind, mainBP, config.URI, config.ID, config.Version)
	if err != nil {
		return errors.Wrapf(err, "invalid %s", kind)
	}

	bpDesc := mainBP.Descriptor()
	for _, deprecatedAPI := range bldr.LifecycleDescriptor().APIs.Buildpack.Deprecated {
		if deprecatedAPI.Equal(bpDesc.API()) {
			c.logger.Warnf(
				"%s %s is using deprecated Buildpacks API version %s",
				cases.Title(language.AmericanEnglish).String(kind),
				style.Symbol(bpDesc.Info().FullName()),
				style.Symbol(bpDesc.API().String()),
			)
			break
		}
	}

	// Fixes 1453
	sort.Slice(depBPs, func(i, j int) bool {
		compareID := strings.Compare(depBPs[i].Descriptor().Info().ID, depBPs[j].Descriptor().Info().ID)
		if compareID == 0 {
			return strings.Compare(depBPs[i].Descriptor().Info().Version, depBPs[j].Descriptor().Info().Version) <= 0
		}
		return compareID < 0
	})

	switch kind {
	case buildpack.KindBuildpack:
		bldr.AddBuildpacks(mainBP, depBPs)
	case buildpack.KindExtension:
		// Extensions can't be composite
		bldr.AddExtension(mainBP)
	default:
		return fmt.Errorf("unknown module kind: %s", kind)
	}
	return nil
}

func validateModule(kind string, module buildpack.BuildModule, source, expectedID, expectedVersion string) error {
	info := module.Descriptor().Info()
	if expectedID != "" && info.ID != expectedID {
		return fmt.Errorf(
			"%s from URI %s has ID %s which does not match ID %s from builder config",
			kind,
			style.Symbol(source),
			style.Symbol(info.ID),
			style.Symbol(expectedID),
		)
	}

	if expectedVersion != "" && info.Version != expectedVersion {
		return fmt.Errorf(
			"%s from URI %s has version %s which does not match version %s from builder config",
			kind,
			style.Symbol(source),
			style.Symbol(info.Version),
			style.Symbol(expectedVersion),
		)
	}

	return nil
}

func uriFromLifecycleVersion(version semver.Version, os string, architecture string) string {
	arch := "x86-64"

	if os == "windows" {
		return fmt.Sprintf("https://github.com/buildpacks/lifecycle/releases/download/v%s/lifecycle-v%s+windows.%s.tgz", version.String(), version.String(), arch)
	}

	if architecture == "arm64" {
		arch = architecture
	}

	return fmt.Sprintf("https://github.com/buildpacks/lifecycle/releases/download/v%s/lifecycle-v%s+linux.%s.tgz", version.String(), version.String(), arch)
}
