package client

import (
	"context"
	"fmt"

	gccrName "github.com/google/go-containerregistry/pkg/name"
)

// RemoveManifest implements commands.PackClient.
func (c *Client) RemoveManifest(ctx context.Context, name string, images []string) error {
	imgIndex, err := c.indexFactory.FindIndex(name)
	if err != nil {
		return err
	}

	for _, image := range images {
		_, err := gccrName.ParseReference(image)
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
