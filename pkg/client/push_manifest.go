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

	if err = idx.Push(parseFalgsForImgUtil(opts)...); err != nil {
		return err
	}

	if !opts.Purge {
		fmt.Printf("successfully pushed index: '%s'\n", index)
		return nil
	}

	return idx.Delete()
}

func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions []imgutil.IndexPushOption) {
	idxOptions = append(idxOptions, imgutil.WithInsecure(opts.Insecure))
	switch opts.Format {
	case "oci":
		return append(idxOptions, imgutil.WithFormat(types.OCIImageIndex))
	case "v2s2":
		return append(idxOptions, imgutil.WithFormat(types.DockerManifestList))
	default:
		return idxOptions
	}
}
