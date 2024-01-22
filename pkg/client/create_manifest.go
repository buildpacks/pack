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
	Insecure, Publish, all bool
}

// CreateManifest implements commands.PackClient.
func (c *Client) CreateManifest(ctx context.Context, name string, images []string, opts CreateManifestOptions) (err error) {
	index, err := c.indexFactory.CreateIndex(name, parseOptsToIndexOptions(opts)...)
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
		fmt.Printf("%s successfully created", name)
	}

	if opts.Publish {
		var format types.MediaType
		switch opts.Format {
		case "oci":
			format = types.OCIImageIndex
		default:
			format = types.DockerManifestList
		}
		index.Push(imgutil.WithInsecure(opts.Insecure), imgutil.WithFormat(format))
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
