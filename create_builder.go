package pack

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
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

	baseImage, err := c.imageFetcher.Fetch(ctx, opts.BuilderConfig.Stack.BuildImage, !opts.Publish, !opts.NoPull)
	if err != nil {
		return err
	}

	builderImage, err := builder.New(baseImage, opts.BuilderName)
	if err != nil {
		return err
	}

	if builderImage.StackID != opts.BuilderConfig.Stack.ID {
		return fmt.Errorf("stack '%s' from builder config is incompatible with stack '%s' from build image", opts.BuilderConfig.Stack.ID, builderImage.StackID)
	}

	for _, b := range opts.BuilderConfig.Buildpacks {
		fetchedBuildpack, err := c.buildpackFetcher.FetchBuildpack(b.URI)
		if err != nil {
			return err
		}
		fetchedBuildpack.Latest = b.Latest
		if b.ID != "" && fetchedBuildpack.ID != b.ID {
			return fmt.Errorf("buildpack from uri '%s' has id '%s' which does not match id '%s' from builder config", b.URI, fetchedBuildpack.ID, b.ID)
		}

		if err := builderImage.AddBuildpack(fetchedBuildpack); err != nil {
			return err
		}
	}
	builderImage.SetOrder(opts.BuilderConfig.Groups)
	builderImage.SetStackInfo(opts.BuilderConfig.Stack)
	return builderImage.Save()
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
