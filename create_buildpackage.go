package pack

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/buildpackage"
	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/style"
)

type CreatePackageOptions struct {
	Name    string
	Config  buildpackage.Config
	Publish bool
}

func (c *Client) CreatePackage(ctx context.Context, opts CreatePackageOptions) error {
	image, err := c.imageFactory.NewImage(opts.Name, !opts.Publish)
	if err != nil {
		return errors.Wrapf(err, "creating image")
	}

	err = dist.SetLabel(image, buildpackage.MetadataLabel, &buildpackage.Metadata{
		BuildpackInfo: opts.Config.Default,
		Stacks:        opts.Config.Stacks,
	})
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "create-package")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	for _, bc := range opts.Config.Blobs {
		blob, err := c.downloader.Download(ctx, bc.URI)
		if err != nil {
			return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(bc.URI))
		}

		bp, err := dist.NewBuildpack(blob)
		if err != nil {
			return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bc.URI))
		}

		bpLayerTar, err := dist.BuildpackLayer(tmpDir, 0, 0, bp)
		if err != nil {
			return err
		}

		if err := image.AddLayer(bpLayerTar); err != nil {
			return errors.Wrapf(err, "adding layer tar for buildpack %s:%s", style.Symbol(bp.Descriptor().Info.ID), style.Symbol(bp.Descriptor().Info.Version))
		}
	}

	_, err = image.Save()
	return err
}
