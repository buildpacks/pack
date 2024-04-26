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
		ref, err := gccrName.NewDigest(image, gccrName.WeakValidation, gccrName.Insecure)
		if err != nil {
			errs = append(errs, fmt.Errorf(`invalid instance "%s": %v`, image, err))
		}

		if err = imgIndex.RemoveManifest(ref); err != nil {
			errs = append(errs, err)
		}

		if err = imgIndex.SaveDir(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		fmt.Printf("Successfully removed images from index: '%s' \n", name)
	}

	return errs
}
