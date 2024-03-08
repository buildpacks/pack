package fakes

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/buildpacks/pack/pkg/client"
)

type FakeBuildpackPackager struct {
	CreateCalledWithOptions client.PackageBuildpackOptions
}

// PackageMultiArchBuildpack implements commands.BuildpackPackager.
func (*FakeBuildpackPackager) PackageMultiArchBuildpack(ctx context.Context, opts client.PackageBuildpackOptions) error {
	panic("unimplemented")
}

// IndexManifest implements commands.BuildpackPackager.
func (*FakeBuildpackPackager) IndexManifest(ctx context.Context, ref name.Reference) (*v1.IndexManifest, error) {
	panic("unimplemented")
}

func (c *FakeBuildpackPackager) PackageBuildpack(ctx context.Context, opts client.PackageBuildpackOptions) error {
	c.CreateCalledWithOptions = opts

	return nil
}
