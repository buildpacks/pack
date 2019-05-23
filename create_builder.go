package pack

import (
	"context"
	"fmt"

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

	lifecycleVersion, err := processLifecycleVersion(opts.BuilderConfig.Lifecycle.Version)
	if err != nil {
		return errors.Wrap(err, "invalid builder config")
	}

	if err := c.validateRunImageConfig(ctx, opts); err != nil {
		return err
	}

	baseImage, err := c.imageFetcher.Fetch(ctx, opts.BuilderConfig.Stack.BuildImage, !opts.Publish, !opts.NoPull)
	if err != nil {
		return err
	}

	c.logger.Verbose("Creating builder %s from build-image %s", style.Symbol(opts.BuilderName), style.Symbol(baseImage.Name()))
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
		fetchedBuildpack, err := c.buildpackFetcher.FetchBuildpack(b.URI)
		if err != nil {
			return err
		}
		fetchedBuildpack.Latest = b.Latest
		if fetchedBuildpack.ID != b.ID {
			return fmt.Errorf("buildpack from URI '%s' has ID '%s' which does not match ID '%s' from builder config", b.URI, fetchedBuildpack.ID, b.ID)
		}

		if fetchedBuildpack.Version != b.Version {
			return fmt.Errorf("buildpack from URI '%s' has version '%s' which does not match version '%s' from builder config", b.URI, fetchedBuildpack.Version, b.Version)
		}

		if err := builderImage.AddBuildpack(fetchedBuildpack); err != nil {
			return err
		}
	}

	if err := builderImage.SetOrder(opts.BuilderConfig.Groups); err != nil {
		return errors.Wrap(err, "builder config has invalid groups")
	}

	builderImage.SetStackInfo(opts.BuilderConfig.Stack)

	lifecycleMd, err := c.lifecycleFetcher.Fetch(lifecycleVersion, opts.BuilderConfig.Lifecycle.URI)
	if err != nil {
		return errors.Wrap(err, "fetching lifecycle")
	}

	if err := builderImage.SetLifecycle(lifecycleMd); err != nil {
		return errors.Wrap(err, "setting lifecycle")
	}

	return builderImage.Save()
}

func processLifecycleVersion(version string) (*semver.Version, error) {
	if version == "" {
		return nil, nil
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return nil, errors.Wrap(err, "lifecycle.version must be a valid semver")
	}
	return v, nil
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
	return nil
}

func (c *Client) validateRunImageConfig(ctx context.Context, opts CreateBuilderOptions) error {
	var runImages []imgutil.Image
	for _, i := range append([]string{opts.BuilderConfig.Stack.RunImage}, opts.BuilderConfig.Stack.RunImageMirrors...) {
		img, err := c.imageFetcher.Fetch(ctx, i, true, false)
		if err != nil {
			if errors.Cause(err) != image.ErrNotFound {
				return err
			}
		} else {
			runImages = append(runImages, img)
			continue
		}

		img, err = c.imageFetcher.Fetch(ctx, i, false, false)
		if err != nil {
			if errors.Cause(err) != image.ErrNotFound {
				return err
			}
			c.logger.Info("Warning: run image %s is not accessible", style.Symbol(i))
		} else {
			runImages = append(runImages, img)
		}
	}

	for _, image := range runImages {
		stackID, err := image.Label("io.buildpacks.stack.id")
		if err != nil {
			return err
		}

		if stackID != opts.BuilderConfig.Stack.ID {
			return fmt.Errorf(
				"stack %s from builder config is incompatible with stack %s from run image %s",
				style.Symbol(opts.BuilderConfig.Stack.ID),
				style.Symbol(stackID),
				style.Symbol(image.Name()),
			)
		}
	}

	return nil
}
