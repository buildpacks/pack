package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
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
		return err
	}

	idx, err := local.NewIndex(opts.Index, opts.Path, local.WithManifest(indexManifest))
	if err != nil {
		return err
	}

	err = idx.AnnotateManifest(
		opts.Manifest,
		local.AnnotateFields{
			Architecture: opts.Architecture,
			OS:           opts.OS,
			Variant:      opts.Variant,
		})
	if err != nil {
		return err
	}

	return nil
}
