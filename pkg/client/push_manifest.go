package client

import (
	"context"
	"github.com/buildpacks/imgutil"
)

type PushManifestOptions struct {
	Format string
	Insecure, Purge bool
}
// PushManifest implements commands.PackClient.
func (c *Client) PushManifest(ctx context.Context, index string, opts PushManifestOptions) (imageID string, err error) {
	manifestList, err := c.runtime.LookupImageIndex(index)
	if err != nil {
		return
	}

	_, list, err := c.runtime.LoadFromImage(manifestList.ID())
	if err != nil {
		return
	}

	_, _, err = list.Push(ctx, parseFalgsForImgUtil(opts))

	if err == nil && opts.Purge {
		c.runtime.RemoveManifests(ctx, []string{manifestList.ID()})
	}

	return imageID, err
}

func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions imgutil.IndexOptions) {
	return idxOptions
}
