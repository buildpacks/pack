package pack

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/style"
)

type CreateBuilderOptions struct {
	BuilderName   string
	BuilderConfig builder.Config
	Publish       bool
	NoPull        bool
}

func (c *Client) CreateBuilder(ctx context.Context, opts CreateBuilderOptions) error {
	if err := validateBuilderConfig(opts.BuilderConfig); err != nil {
		return errors.Wrap(err, "invalid builder config")
	}

	if err := c.validateRunImageConfig(ctx, opts); err != nil {
		return err
	}

	baseImage, err := c.imageFetcher.Fetch(ctx, opts.BuilderConfig.Stack.BuildImage, !opts.Publish, !opts.NoPull)
	if err != nil {
		return err
	}

	c.logger.Debugf("Creating builder %s from build-image %s", style.Symbol(opts.BuilderName), style.Symbol(baseImage.Name()))
	builderImage, err := builder.New(baseImage, opts.BuilderName)
	if err != nil {
		return errors.Wrap(err, "invalid build-image")
	}

	builderImage.SetDescription(opts.BuilderConfig.Description)

	if builderImage.StackID != opts.BuilderConfig.Stack.ID {
		return fmt.Errorf(
			"stack %s from builder config is incompatible with stack %s from build image",
			style.Symbol(opts.BuilderConfig.Stack.ID),
			style.Symbol(builderImage.StackID),
		)
	}

	for _, b := range opts.BuilderConfig.Buildpacks {
		err := ensureBPSupport(b.URI)
		if err != nil {
			return err
		}

		blob, err := c.downloader.Download(b.URI)
		if err != nil {
			return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(b.URI))
		}

		fetchedBuildpack, err := builder.NewBuildpack(blob)
		if err != nil {
			return errors.Wrap(err, "creating buildpack")
		}

		if b.ID != "" && fetchedBuildpack.Descriptor().Info.ID != b.ID {
			return fmt.Errorf("buildpack from URI '%s' has ID '%s' which does not match ID '%s' from builder config", b.URI, fetchedBuildpack.Descriptor().Info.ID, b.ID)
		}

		if b.Version != "" && fetchedBuildpack.Descriptor().Info.Version != b.Version {
			return fmt.Errorf("buildpack from URI '%s' has version '%s' which does not match version '%s' from builder config", b.URI, fetchedBuildpack.Descriptor().Info.Version, b.Version)
		}

		builderImage.AddBuildpack(fetchedBuildpack)
	}

	builderImage.SetOrder(opts.BuilderConfig.Order)
	builderImage.SetStackInfo(opts.BuilderConfig.Stack)

	lifecycle, err := c.fetchLifecycle(opts.BuilderConfig.Lifecycle)
	if err != nil {
		return errors.Wrap(err, "fetch lifecycle")
	}

	if err := builderImage.SetLifecycle(lifecycle); err != nil {
		return errors.Wrap(err, "setting lifecycle")
	}

	return builderImage.Save()
}

func (c *Client) fetchLifecycle(config builder.LifecycleConfig) (builder.Lifecycle, error) {
	if config.Version != "" && config.URI != "" {
		return nil, errors.Errorf(
			"%s can only declare %s or %s, not both",
			style.Symbol("lifecycle"), style.Symbol("version"), style.Symbol("uri"),
		)
	}

	var uri string
	if config.Version != "" {
		v, err := semver.NewVersion(config.Version)
		if err != nil {
			return nil, errors.Wrapf(err, "%s must be a valid semver", style.Symbol("lifecycle.version"))
		}

		uri = uriFromLifecycleVersion(*v)
	} else {
		uri = config.URI
	}

	b, err := c.downloader.Download(uri)
	if err != nil {
		return nil, errors.Wrap(err, "downloading lifecycle")
	}

	lifecycle, err := builder.NewLifecycle(b)
	if err != nil {
		return nil, errors.Wrap(err, "invalid lifecycle")
	}

	return lifecycle, nil
}

func uriFromLifecycleVersion(version semver.Version) string {
	return fmt.Sprintf("https://github.com/buildpack/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz", version.String(), version.String())
}

func validateBuilderConfig(conf builder.Config) error {
	if conf.Stack.ID == "" {
		return errors.New("stack.id is required")
	}

	if conf.Stack.BuildImage == "" {
		return errors.New("stack.build-image is required")
	}

	if conf.Stack.RunImage == "" {
		return errors.New("stack.run-image is required")
	}

	if runtime.GOOS == "windows" {
		for _, bp := range conf.Buildpacks {
			if filepath.Ext(bp.URI) != ".tgz" {
				return fmt.Errorf("buildpack %s: directory-based buildpacks are not currently supported on Windows", style.Symbol(bp.ID))
			}
		}
	}

	return nil
}

func (c *Client) validateRunImageConfig(ctx context.Context, opts CreateBuilderOptions) error {
	var runImages []imgutil.Image
	for _, i := range append([]string{opts.BuilderConfig.Stack.RunImage}, opts.BuilderConfig.Stack.RunImageMirrors...) {
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

		if stackID != opts.BuilderConfig.Stack.ID {
			return fmt.Errorf(
				"stack %s from builder config is incompatible with stack %s from run image %s",
				style.Symbol(opts.BuilderConfig.Stack.ID),
				style.Symbol(stackID),
				style.Symbol(img.Name()),
			)
		}
	}

	return nil
}
