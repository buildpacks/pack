package pack

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver"
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/buildpack"

	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/internal/style"
)

// CreateBuilderOptions are options passed into CreateBuilder
type CreateBuilderOptions struct {
	BuilderName string
	Config      pubbldr.Config
	Publish     bool
	NoPull      bool
	Registry    string
}

// CreateBuilder allows users to create a builder
func (c *Client) CreateBuilder(ctx context.Context, opts CreateBuilderOptions) error {
	if err := c.validateConfig(ctx, opts); err != nil {
		return err
	}

	builder, err := c.createBaseBuilder(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "failed to create builder")
	}

	if err := c.addBuildpacksToBuilder(ctx, opts, builder); err != nil {
		return errors.Wrap(err, "failed to add buildpacks to builder")
	}

	builder.SetOrder(opts.Config.Order)
	builder.SetStack(opts.Config.Stack)

	return builder.Save(c.logger)
}

func (c *Client) validateConfig(ctx context.Context, opts CreateBuilderOptions) error {
	if err := opts.Config.Validate(); err != nil {
		return errors.Wrap(err, "invalid builder config")
	}

	if err := c.validateRunImageConfig(ctx, opts); err != nil {
		return errors.Wrap(err, "invalid run image config")
	}

	return nil
}

func (c *Client) validateRunImageConfig(ctx context.Context, opts CreateBuilderOptions) error {
	var runImages []imgutil.Image
	for _, i := range append([]string{opts.Config.Stack.RunImage}, opts.Config.Stack.RunImageMirrors...) {
		if !opts.Publish {
			img, err := c.imageFetcher.Fetch(ctx, i, true, false)
			if err != nil {
				if errors.Cause(err) != image.ErrNotFound {
					return err
				}
			} else {
				runImages = append(runImages, img)
				continue
			}
		}

		img, err := c.imageFetcher.Fetch(ctx, i, false, false)
		if err != nil {
			if errors.Cause(err) != image.ErrNotFound {
				return err
			}
			c.logger.Warnf("run image %s is not accessible", style.Symbol(i))
		} else {
			runImages = append(runImages, img)
		}
	}

	for _, img := range runImages {
		stackID, err := img.Label("io.buildpacks.stack.id")
		if err != nil {
			return err
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

	return nil
}

func (c *Client) createBaseBuilder(ctx context.Context, opts CreateBuilderOptions) (*builder.Builder, error) {
	baseImage, err := c.imageFetcher.Fetch(ctx, opts.Config.Stack.BuildImage, !opts.Publish, !opts.NoPull)
	if err != nil {
		return nil, errors.Wrap(err, "fetch build image")
	}

	c.logger.Debugf("Creating builder %s from build-image %s", style.Symbol(opts.BuilderName), style.Symbol(baseImage.Name()))
	bldr, err := builder.New(baseImage, opts.BuilderName)
	if err != nil {
		return nil, errors.Wrap(err, "invalid build-image")
	}

	bldr.SetDescription(opts.Config.Description)

	if bldr.StackID != opts.Config.Stack.ID {
		return nil, fmt.Errorf(
			"stack %s from builder config is incompatible with stack %s from build image",
			style.Symbol(opts.Config.Stack.ID),
			style.Symbol(bldr.StackID),
		)
	}

	lifecycle, err := c.fetchLifecycle(ctx, opts.Config.Lifecycle)
	if err != nil {
		return nil, errors.Wrap(err, "fetch lifecycle")
	}

	if err := bldr.SetLifecycle(lifecycle); err != nil {
		return nil, errors.Wrap(err, "setting lifecycle")
	}

	return bldr, nil
}

func (c *Client) fetchLifecycle(ctx context.Context, config pubbldr.LifecycleConfig) (builder.Lifecycle, error) {
	if config.Version != "" && config.URI != "" {
		return nil, errors.Errorf(
			"%s can only declare %s or %s, not both",
			style.Symbol("lifecycle"), style.Symbol("version"), style.Symbol("uri"),
		)
	}

	var uri string
	switch {
	case config.Version != "":
		v, err := semver.NewVersion(config.Version)
		if err != nil {
			return nil, errors.Wrapf(err, "%s must be a valid semver", style.Symbol("lifecycle.version"))
		}

		uri = uriFromLifecycleVersion(*v)
	case config.URI != "":
		uri = config.URI
	default:
		uri = uriFromLifecycleVersion(*semver.MustParse(builder.DefaultLifecycleVersion))
	}

	b, err := c.downloader.Download(ctx, uri)
	if err != nil {
		return nil, errors.Wrap(err, "downloading lifecycle")
	}

	lifecycle, err := builder.NewLifecycle(b)
	if err != nil {
		return nil, errors.Wrap(err, "invalid lifecycle")
	}

	return lifecycle, nil
}

func (c *Client) addBuildpacksToBuilder(ctx context.Context, opts CreateBuilderOptions, bldr *builder.Builder) error {
	for _, b := range opts.Config.Buildpacks.Buildpacks() {
		locatorType, err := buildpack.GetLocatorType(b.URI, []dist.BuildpackInfo{})
		if err != nil {
			return err
		}

		switch locatorType {
		case buildpack.RegistryLocator:
			registryCache, err := c.getRegistry(c.logger, opts.Registry)
			if err != nil {
				return errors.Wrapf(err, "invalid registry '%s'", opts.Registry)
			}

			registryBp, err := registryCache.LocateBuildpack(b.URI)
			if err != nil {
				return errors.Wrapf(err, "locating in registry %s", style.Symbol(b.URI))
			}

			mainBP, depBPs, err := extractPackagedBuildpacks(ctx, registryBp.Address, c.imageFetcher, opts.Publish, opts.NoPull)
			if err != nil {
				return errors.Wrapf(err, "extracting from registry %s", style.Symbol(b.URI))
			}

			for _, bp := range append([]dist.Buildpack{mainBP}, depBPs...) {
				bldr.AddBuildpack(bp)
			}
		default:
			err := ensureBPSupport(b.URI)
			if err != nil {
				return err
			}

			blob, err := c.downloader.Download(ctx, b.URI)
			if err != nil {
				return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(b.URI))
			}

			layerWriterFactory, err := layer.NewWriterFactory(bldr.Image())
			if err != nil {
				return errors.Wrapf(err, "get tar writer factory for image %s", style.Symbol(bldr.Name()))
			}
			fetchedBp, err := dist.BuildpackFromRootBlob(blob, layerWriterFactory)
			if err != nil {
				return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(b.URI))
			}

			err = validateBuildpack(fetchedBp, b.URI, b.ID, b.Version)
			if err != nil {
				return errors.Wrap(err, "invalid buildpack")
			}

			bldr.AddBuildpack(fetchedBp)
		}
	}

	for _, pkg := range opts.Config.Buildpacks.Packages() {
		locatorType, err := buildpack.GetLocatorType(pkg.ImageName, []dist.BuildpackInfo{})
		if err != nil {
			return err
		}

		switch locatorType {
		case buildpack.PackageLocator:
			mainBP, depBPs, err := extractPackagedBuildpacks(ctx, pkg.ImageName, c.imageFetcher, opts.Publish, opts.NoPull)
			if err != nil {
				return err
			}

			for _, bp := range append([]dist.Buildpack{mainBP}, depBPs...) {
				bldr.AddBuildpack(bp)
			}
		default:
			return fmt.Errorf("invalid image format: %s", pkg.ImageName)
		}
	}

	return nil
}

func validateBuildpack(bp dist.Buildpack, source, expectedID, expectedBPVersion string) error {
	if expectedID != "" && bp.Descriptor().Info.ID != expectedID {
		return fmt.Errorf(
			"buildpack from URI %s has ID %s which does not match ID %s from builder config",
			style.Symbol(source),
			style.Symbol(bp.Descriptor().Info.ID),
			style.Symbol(expectedID),
		)
	}

	if expectedBPVersion != "" && bp.Descriptor().Info.Version != expectedBPVersion {
		return fmt.Errorf(
			"buildpack from URI %s has version %s which does not match version %s from builder config",
			style.Symbol(source),
			style.Symbol(bp.Descriptor().Info.Version),
			style.Symbol(expectedBPVersion),
		)
	}

	return nil
}

func uriFromLifecycleVersion(version semver.Version) string {
	return fmt.Sprintf("https://github.com/buildpacks/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz", version.String(), version.String())
}
