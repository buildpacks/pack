package client

import "context"

type RemoveManifestOptions struct {
	Index string
}

func (c *Client) RemoveManifest(ctx context.Context, opts RemoveManifestOptions) error {
	return nil
}
