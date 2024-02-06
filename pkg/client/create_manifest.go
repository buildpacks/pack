package client

import (
	"context"
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/index"
	ggcrName "github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type CreateManifestOptions struct {
	Format, Registry       string
	Insecure, Publish, All bool
}

// CreateManifest implements commands.PackClient.
func (c *Client) CreateManifest(ctx context.Context, name string, images []string, opts CreateManifestOptions) (err error) {
	ops := parseOptsToIndexOptions(opts)
	_, err = c.indexFactory.LoadIndex(name, ops...)
	if err == nil {
		return fmt.Errorf("image index with name: '%s' exists", name)
	}

	_, err = c.indexFactory.CreateIndex(name, ops...)
	if err != nil {
		return
	}

	index, err := c.indexFactory.LoadIndex(name, ops...)
	if err != nil {
		return err
	}

	for _, img := range images {
		ref, err := ggcrName.ParseReference(img)
		if err != nil {
			return err
		}
		if err = index.Add(ref, imgutil.WithAll(opts.All)); err != nil {
			return err
		}
	}

	err = index.Save()
	if err != nil {
		return err
	}

	fmt.Printf("successfully created index: '%s'\n", name)
	if opts.Publish {
		var format types.MediaType
		switch opts.Format {
		case "oci":
			format = types.OCIImageIndex
		default:
			format = types.DockerManifestList
		}

		err = index.Push(imgutil.WithInsecure(opts.Insecure), imgutil.WithFormat(format))
		if err != nil {
			return err
		}

		fmt.Printf("successfully pushed '%s' to registry \n", name)
	}

	return err
}

func parseOptsToIndexOptions(opts CreateManifestOptions) (idxOpts []index.Option) {
	var format types.MediaType
	switch opts.Format {
	case "oci":
		format = types.OCIImageIndex
	default:
		format = types.DockerManifestList
	}
	return []index.Option{
		index.WithFormat(format),
		index.WithInsecure(opts.Insecure),
	}
}
