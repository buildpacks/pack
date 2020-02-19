package fakes

import (
	"context"

	"github.com/buildpacks/pack"
)

type FakeBuildpackPackager struct {
	CreateCalledWithOptions pack.PackageBuildpackOptions
}

func (c *FakeBuildpackPackager) PackageBuildpack(ctx context.Context, opts pack.PackageBuildpackOptions) error {
	c.CreateCalledWithOptions = opts

	return nil
}
