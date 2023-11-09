package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/buildpacks/imgutil"

	packErrors "github.com/buildpacks/pack/pkg/errors"
)

type CreateManifestOptions struct {
	Format, Registry              string
	Insecure, Publish, amend, all bool
}

// CreateManifest implements commands.PackClient.
func (c *Client) CreateManifest(ctx context.Context, name string, images []string, opts CreateManifestOptions) (imageID string, err error) {
	index, err := c.indexFactory.NewIndex(name, parseOptsToIndexOptions(opts))
	if err != nil {
		return
	}
	index.Create()
	if err != nil {
		return
	}
	if imageID, err = index.Save(name, c.runtime.ImageType(opts.Format)); err != nil {
		if errors.Is(err, packErrors.ErrDuplicateName) && opts.amend {
			_, err := c.runtime.LookupImageIndex(name)
			if err != nil {
				fmt.Printf("no list named %q found: %v", name, err)
			}

			if index == nil {
				return imageID, fmt.Errorf("--amend specified but no matching manifest list found with name %q", name)
			}
		} else {
			return
		}
	}

	for _, img := range images {
		ref, err := c.runtime.ParseReference(img)
		if err != nil {
			return imageID, err
		}
		if localRef, _, err := c.imageFactory.FindImage(img); err == nil {
			ref = localRef
		}
		if _, err = index.Add(ctx, ref, opts.all); err != nil {
			return imageID, err
		}
	}

	imageID, err = index.Save(name, c.runtime.ImageType(opts.Format))
	if err == nil {
		fmt.Printf("%s\n", imageID)
	}

	if opts.Publish {

	}

	return imageID, err
}

func parseOptsToIndexOptions(opts CreateManifestOptions) (idxOpts imgutil.IndexOptions) {
	return idxOpts
}