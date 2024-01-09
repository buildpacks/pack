package client

import (
	"context"

	"github.com/buildpacks/imgutil"
)

type PushManifestOptions struct {
	Format          string
	Insecure, Purge bool
}

// PushManifest implements commands.PackClient.
func (c *Client) PushManifest(ctx context.Context, index string, opts PushManifestOptions) (err error) {
	idx, err := c.indexFactory.LoadIndex(index)
	if err != nil {
		return 
	}

	err = idx.Push()
	if err != nil {
		return
	}

	if opts.Purge {
		if err = idx.Delete(); err != nil {
			return
		}
	}

	return
}

func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions []imgutil.IndexOption) {
	return []imgutil.IndexOption{
		imgutil.WithFormat(opts.Format),
		imgutil.WithInsecure(opts.Insecure),
	}
}
