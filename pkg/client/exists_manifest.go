package client

import (
	"context"

	// "github.com/buildpacks/imgutil"
	"github.com/pkg/errors"
)

func (c *Client) ExistsManifest(ctx context.Context, image string) error {

	if _, err := c.runtime.LookupImageIndex(image); err != nil {
		return errors.Errorf("image '%s' is not found", image)
	}

	return nil
}
