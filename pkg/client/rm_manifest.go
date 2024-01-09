package client

import (
	"context"
	"fmt"

	gccrName "github.com/google/go-containerregistry/pkg/name"
)

// RemoveManifest implements commands.PackClient.
func (c *Client) RemoveManifest(ctx context.Context, name string, images []string) (errs []error) {
	imgIndex, err := c.indexFactory.LoadIndex(name)
	if err != nil {
		return append(errs, err)
	}

	for _, image := range images {
		ref, err := gccrName.ParseReference(image)
		if err != nil {
			errs = append(errs, fmt.Errorf(`Invalid instance "%s": %v`, image, err))
		}
		if err := imgIndex.Remove(ref.Context().Digest(ref.Identifier())); err != nil {
			errs = append(errs, err)
		}
		fmt.Printf("Successfully removed %s from %s", image, name)
	}

	return errs
}
