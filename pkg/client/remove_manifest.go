package client

import (
	"context"
)

// DeleteManifest implements commands.PackClient.
func (c *Client) DeleteManifest(ctx context.Context, names []string) []error {
	var errs []error
	for _, name := range names {
		imgIndex, err := c.indexFactory.FindIndex(name)
		if err != nil {
			errs = append(errs, err)
		}

		imgIndex.Delete()
	}

	return errs
}
