package client

import (
	"context"
	"fmt"

	"github.com/buildpacks/imgutil"
	ggcrName "github.com/google/go-containerregistry/pkg/name"
)

type CreateManifestOptions struct {
	Format, Registry              string
	Insecure, Publish, all bool
}

// CreateManifest implements commands.PackClient.
func (c *Client) CreateManifest(ctx context.Context, name string, images []string, opts CreateManifestOptions) (imageID string, err error) {
	index, err := c.indexFactory.FindIndex(name, parseOptsToIndexOptions(opts))
	if err != nil {
		return
	}

	imageID = index.RepoName()

	for _, img := range images {
		ref, err := ggcrName.ParseReference(img)
		if err != nil {
			return imageID, err
		}
		if opts.all {
			if _, err = index.Add(ref, imgutil.WithAll()); err != nil {
				return imageID, err
			}
		} else {
			if _, err = index.Add(ref); err != nil {
				return imageID, err
			}
		}
	}

	err = index.Save()
	if err == nil {
		fmt.Printf("%s\n", imageID)
	}

	if opts.Publish {
		index.Push()
	}

	return imageID, err
}

func parseOptsToIndexOptions(opts CreateManifestOptions) (idxOpts []imgutil.IndexOption) {
	return idxOpts
}
