package pack

import (
	"context"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

type CreatePackageOptions struct {
	Name    string
	Config  buildpackage.Config
	Publish bool
	NoPull  bool
}

func (c *Client) CreatePackage(ctx context.Context, opts CreatePackageOptions) error {
	packageBuilder := buildpackage.NewBuilder(c.imageFactory)

	for _, bc := range opts.Config.Buildpacks {
		blob, err := c.downloader.Download(ctx, bc.URI)
		if err != nil {
			return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(bc.URI))
		}

		bp, err := dist.BuildpackFromRootBlob(blob)
		if err != nil {
			return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bc.URI))
		}

		packageBuilder.AddBuildpack(bp)
	}

	for _, pkg := range opts.Config.Packages {
		if err := addPackageBuildpacks(ctx, pkg.Ref, packageBuilder, c.imageFetcher, opts.Publish, opts.NoPull); err != nil {
			return err
		}
	}

	packageBuilder.SetDefaultBuildpack(opts.Config.Default)

	for _, s := range opts.Config.Stacks {
		packageBuilder.AddStack(s)
	}

	_, err := packageBuilder.Save(opts.Name, opts.Publish)
	if err != nil {
		return errors.Wrapf(err, "saving image")
	}

	return err
}
