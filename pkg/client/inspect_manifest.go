package client

import (
	"context"
	"fmt"
)

// InspectManifest implements commands.PackClient.
func (c *Client) InspectManifest(ctx context.Context, name string) error {
	idx, err := c.indexFactory.FindIndex(name)
	if err != nil {
		return err
	}

	mfest, err := idx.Inspect()
	if err != nil {
		return err
	}

	fmt.Println(mfest)
	return nil
}
