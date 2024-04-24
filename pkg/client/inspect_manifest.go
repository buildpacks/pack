package client

import (
	"context"
	"fmt"
)

// InspectManifest implements commands.PackClient.
func (c *Client) InspectManifest(ctx context.Context, name string) (err error) {
	idx, err := c.indexFactory.FindIndex(name)
	if err != nil {
		return err
	}

	if mfest, err := idx.Inspect(); err == nil {
		fmt.Println(mfest)
	}

	return err
}
