package client

import (
	"context"
	"fmt"
)

// RemoveManifest implements commands.PackClient.
func (c *Client) RemoveManifest(ctx context.Context, name string, images []string) error {
	imgIndex, err := c.runtime.LookupImageIndex(name)
	if err != nil {
		return err
	}

	for _, image := range images {
		d, err := c.runtime.ParseDigest(image)
		if err != nil {
			fmt.Errorf(`Invalid instance "%s": %v`, image, err)
		}
		if err := imgIndex.Remove(d); err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", imgIndex.ID(), d.String())
	}
	
	return nil
}
