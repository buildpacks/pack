package fakes

import (
	"context"

	"github.com/buildpacks/pack/pkg/client"
)

type FakeBuildpackPackager struct {
	CreateCalledWithOptions client.PackageBuildpackOptions
}

// PackageMultiArchExtension implements commands.ExtensionPackager.
func (c *FakeBuildpackPackager) PackageMultiArchExtension(ctx context.Context, opts client.PackageBuildpackOptions) error {
	c.CreateCalledWithOptions = opts

	return nil
}

func (c *FakeBuildpackPackager) PackageBuildpack(ctx context.Context, opts client.PackageBuildpackOptions) error {
	c.CreateCalledWithOptions = opts

	return nil
}

func (c *FakeBuildpackPackager) PackageMultiArchBuildpack(ctx context.Context, opts client.PackageBuildpackOptions) error {
	c.CreateCalledWithOptions = opts

	return nil
}
