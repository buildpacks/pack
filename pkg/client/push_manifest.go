package client

import (
	"context"
)

type PushManifestOptions struct {
	Format          string
	Insecure, Purge bool
}

// PushManifest implements commands.PackClient.
func (c *Client) PushManifest(ctx context.Context, index string, opts PushManifestOptions) (imageID string, err error) {
	manifestList, err := c.indexFactory.FindIndex(index)
	if err != nil {
		return
	}

	_, err = manifestList.Push(ctx, parseFalgsForImgUtil(opts))

	manifestList.Delete()

	return imageID, err
}

func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions runtime.PushOptions) {
	return idxOptions
}
