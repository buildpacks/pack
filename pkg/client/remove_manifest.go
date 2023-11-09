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
		_, err := c.runtime.ParseReference(image)
		if err != nil {
			fmt.Errorf(`Invalid instance "%s": %v`, image, err)
		}
		if err := imgIndex.Remove(image); err != nil {
			return err
		}
		fmt.Printf("Successfully removed %s from %s", image, name)
	}

	return nil
}