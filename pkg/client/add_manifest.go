package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
)

type AddManifestOptions struct {
	Index    string
	Manifest string
	All      bool
}

func (c *Client) AddManifest(ctx context.Context, opts AddManifestOptions) error {

	err := local.AppendManifest(opts.Index, opts.Manifest)
	if err != nil {
		return err
	}

	return nil
}
