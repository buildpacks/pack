package client

import (
	"context"
)

// DeleteManifest implements commands.PackClient.
func (c *Client) DeleteManifest(ctx context.Context, names []string) (errs []error) {
	for _, name := range names {
		imgIndex, err := c.indexFactory.LoadIndex(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if err := imgIndex.DeleteDir(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		c.logger.Info("Successfully deleted manifest lists")
	}
	return errs
}
