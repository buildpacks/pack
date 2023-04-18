package client

import "context"

type AddManifestOptions struct {
	Index        string
	Manifest     string
	Architecture string
	OS           string
	Variant      string
	All          bool
}

func (c *Client) AddManifest(ctx context.Context, opts AddManifestOptions) error {
	return nil
}
