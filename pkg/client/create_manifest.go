package client

import (
	"context"
	"fmt"
	"sync"

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

	_, err = c.indexFactory.CreateIndex(name, ops...)
	if err != nil {
		return
	}

	index, err := c.indexFactory.LoadIndex(name, ops...)
	if err != nil {
		return err
	}

	var errGroup, _ = errgroup.WithContext(ctx)
	var wg sync.WaitGroup
	for _, img := range images {
		img := img
		wg.Add(1)
		errGroup.Go(func() error {
			return addImage(index, img, &wg, opts)
		})
	}

	wg.Wait()
	if err = errGroup.Wait(); err != nil {
		return err
	}

	err = index.Save()
	if err != nil {
		return err
	}

	fmt.Printf("successfully created index: '%s'\n", name)
	if opts.Publish {
		err = index.Push(imgutil.WithInsecure(opts.Insecure))
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

func addImage(index imgutil.ImageIndex, img string, wg *sync.WaitGroup, opts CreateManifestOptions) error {
	ref, err := ggcrName.ParseReference(img)
	if err != nil {
		return err
	}
	if err = index.Add(ref, imgutil.WithAll(opts.All)); err != nil {
		return err
	}

	wg.Done()
	return nil
}
