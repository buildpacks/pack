package client

import "context"

type PushManifestOptions struct {
	Index string
}

func (c *Client) PushManifest(ctx context.Context, opts PushManifestOptions) error {
	return nil
}
