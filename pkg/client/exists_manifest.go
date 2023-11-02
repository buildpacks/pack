package client

import (
	"context"
	"os"

	"github.com/buildpacks/imgutil"
	packErrors "github.com/buildpacks/pack/pkg/errors"
	"github.com/pkg/errors"
)

func(c *Client) ExistsManifest(ctx context.Context, image string) error {
	index, err := c.indexFactory.NewIndex(image, imgutil.IndexOptions{})
	if err != nil {
		return errors.Errorf("error while initializing index: %s", image)
	}

	if _, err := c.runtime.LookupImageIndex(image); err != nil {
		if errors.Is(err, packErrors.ErrImageUnknown) {
			os.Exit(1)
		} else {
			return errors.Errorf("image '%s' is not found", image)
		}
	}

	return nil
}