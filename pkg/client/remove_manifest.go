package client

import (
	"context"
	"fmt"
)

// DeleteManifest implements commands.PackClient.
func (c *Client) DeleteManifest(ctx context.Context, names []string) []error {
	var errs []error
	for _, name := range names {
		imgIndex, err := c.indexFactory.LoadIndex(name)
		if err != nil {
			errs = append(errs, err)
		}

		if err = imgIndex.Delete(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 {
		fmt.Printf("successfully deleted indexes \n")
	}

	return errs
}
