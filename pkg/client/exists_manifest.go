package client

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
)

func (c *Client) ExistsManifest(ctx context.Context, image string) error {
	if _, err := c.indexFactory.LoadIndex(image); err != nil {
		return errors.Errorf("image '%s' is not found", image)
	}

	fmt.Printf("index '%s' exists \n", image)
	return nil
}
