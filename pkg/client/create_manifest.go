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
func (c *Client) CreateManifest(ctx context.Context, name string, images []string, opts CreateManifestOptions) (err error) {
	index, err := c.indexFactory.FindIndex(name, parseOptsToIndexOptions(opts)...)
	if err != nil {
		return
	}

	for _, img := range images {
		ref, err := ggcrName.ParseReference(img)
		if err != nil {
			return err
		}
		if err = index.Add(ref, imgutil.WithAll(opts.all)); err != nil {
			return err
		}
	}

	err = index.Save()
	if err == nil {
		fmt.Println("%s successfully created", name)
	}

	if opts.Publish {
		index.Push()
	}

	return err
}

func parseOptsToIndexOptions(opts CreateManifestOptions) (idxOpts []imgutil.IndexOption) {
	return []imgutil.IndexOption{
		imgutil.WithFormat(opts.Format),
		imgutil.AddRegistry(opts.Registry),
		imgutil.WithInsecure(opts.Insecure),
	}
}
