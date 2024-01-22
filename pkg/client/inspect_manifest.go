package client

import (
	"context"
)

type InspectManifestOptions struct {
}

// InspectManifest implements commands.PackClient.
func (c *Client) InspectManifest(ctx context.Context, name string) error {
	idx, err := c.indexFactory.LoadIndex(name)
	if err != nil {
		return err
	}

	return idx.Inspect()
}
