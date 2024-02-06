package client

import (
	"context"
	"fmt"

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

	fmt.Printf("successfully pushed index: '%s'\n", index)
	return
}

func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions []imgutil.IndexPushOption) {
	switch opts.Format {
	case "oci":
		return []imgutil.IndexPushOption{
			imgutil.WithFormat(types.OCIImageIndex),
			imgutil.WithInsecure(opts.Insecure),
		}
	case "v2s2":
		return []imgutil.IndexPushOption{
			imgutil.WithFormat(types.DockerManifestList),
			imgutil.WithInsecure(opts.Insecure),
		}
	default:
		return []imgutil.IndexPushOption{
			imgutil.WithInsecure(opts.Insecure),
		}
	}
}
