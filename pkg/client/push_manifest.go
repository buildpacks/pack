package client

import (
	"context"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/v1/types"
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

	err = idx.Push(parseFalgsForImgUtil(opts)...)
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

func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions []imgutil.IndexPushOption) {
	var format types.MediaType
	switch opts.Format {
	case "oci":
		format = types.OCIImageIndex
	default:
		format = types.DockerManifestList
	}

	return []imgutil.IndexPushOption{
		imgutil.WithFormat(format),
		imgutil.WithInsecure(opts.Insecure),
	}
}
