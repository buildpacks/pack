package pack

import (
	"context"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/buildpackage"
	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/style"
)

type CreatePackageOptions struct {
	Name    string
	Config  buildpackage.Config
	Publish bool
}

func (c *Client) CreatePackage(ctx context.Context, opts CreatePackageOptions) error {
	packageBuilder := buildpackage.NewBuilder(c.imageFactory)

	for _, bc := range opts.Config.Blobs {
		blob, err := c.downloader.Download(ctx, bc.URI)
		if err != nil {
			return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(bc.URI))
		}

		bp, err := dist.NewBuildpack(blob)
		if err != nil {
			return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bc.URI))
		}

		packageBuilder.AddBuildpack(bp)
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
