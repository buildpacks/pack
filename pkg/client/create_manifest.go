package client

import "context"

// CreateManifest implements commands.PackClient.
func (*Client) CreateManifest(context.Context, string, []string) (imageID string, err error) {
	panic("unimplemented")
}
