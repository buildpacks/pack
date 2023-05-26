package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
	"github.com/pkg/errors"
)

type AnnotateManifestOptions struct {
	Index        string
	Path         string
	Manifest     string
	Architecture string
	OS           string
	Variant      string
}

func (c *Client) AnnotateManifest(ctx context.Context, opts AnnotateManifestOptions) error {
	indexManifest, err := local.GetIndexManifest(opts.Index, opts.Path)
	if err != nil {
		return errors.Wrapf(err, "Get local index manifest '%s' from path '%s'", opts.Index, opts.Path)
	}

	idx, err := local.NewIndex(opts.Index, opts.Path, local.WithManifest(indexManifest))
	if err != nil {
		return errors.Wrapf(err, "Create local index from '%s' local index manifest", opts.Index)
	}

	err = idx.AnnotateManifest(
		opts.Manifest,
		local.AnnotateFields{
			Architecture: opts.Architecture,
			OS:           opts.OS,
			Variant:      opts.Variant,
		})
	if err != nil {
		return errors.Wrapf(err, "Annotate manifet '%s' of index '%s", opts.Manifest, opts.Index)
	}

	return nil
}
