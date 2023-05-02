package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
)

type AnnotateManifestOptions struct {
	Index        string
	Manifest     string
	Architecture string
	OS           string
	Variant      string
}

func (c *Client) AnnotateManifest(ctx context.Context, opts AnnotateManifestOptions) error {
	err := local.AnnotateManifest(
		opts.Index,
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
