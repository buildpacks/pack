package client

import "context"

type AnnotateManifestOptions struct {
	Index        string
	Manifest     string
	Architecture string
	OS           string
	Variant      string
}

func (c *Client) AnnotateManifest(ctx context.Context, opts AnnotateManifestOptions) error {
	return nil
}
