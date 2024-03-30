package client

import (
	"context"
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/index"
	ggcrName "github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"golang.org/x/sync/errgroup"
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
		return fmt.Errorf("exits in your local storage, use 'pack manifest remove' if you want to delete it")
	}

	if _, err = c.indexFactory.CreateIndex(name, ops...); err != nil {
		return err
	}

	index, err := c.indexFactory.LoadIndex(name, ops...)
	if err != nil {
		return err
	}

	var errGroup, _ = errgroup.WithContext(ctx)
	for _, img := range images {
		img := img
		errGroup.Go(func() error {
			return addImage(index, img, opts)
		})
	}

	if err = errGroup.Wait(); err != nil {
		return err
	}

	if err = index.Save(); err != nil {
		return err
	}

	fmt.Printf("successfully created index: '%s'\n", name)
	if !opts.Publish {
		return nil
	}

	if err = index.Push(imgutil.WithInsecure(opts.Insecure)); err != nil {
		return err
	}

	fmt.Printf("successfully pushed '%s' to registry \n", name)
	return nil
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

func addImage(index imgutil.ImageIndex, img string, opts CreateManifestOptions) error {
	ref, err := ggcrName.ParseReference(img)
	if err != nil {
		return err
	}

	return index.Add(ref, imgutil.WithAll(opts.All))
}
