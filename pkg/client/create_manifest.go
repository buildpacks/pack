package client

import (
	"context"
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/pkg/image"
)

type CreateManifestOptions struct {
	// Image index we want to create
	IndexRepoName string

	// Name of images we wish to add into the image index
	RepoNames []string

	// Media type of the index
	Format string

	// true if we want to publish to an insecure registry
	Insecure bool

	// true if we want to push the index to a registry after creating
	Publish bool
}

// CreateManifest implements commands.PackClient.
func (c *Client) CreateManifest(ctx context.Context, opts CreateManifestOptions) (err error) {
	ops := parseOptsToIndexOptions(opts)

	if c.indexFactory.Exists(opts.IndexRepoName) {
		return errors.New("exits in your local storage, use 'pack manifest remove' if you want to delete it")
	}

	index, err := c.indexFactory.CreateIndex(opts.IndexRepoName, ops...)
	if err != nil {
		return err
	}

	for _, repoName := range opts.RepoNames {
		// TODO same code to add_manifest.go externalize it!
		imageRef, err := name.ParseReference(repoName, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid manifest reference: %s", repoName, err)
		}

		imageToAdd, err := c.imageFetcher.Fetch(ctx, imageRef.Name(), image.FetchOptions{Daemon: false})
		if err != nil {
			return err
		}

		index.AddManifest(imageToAdd.UnderlyingImage())
	}

	if err = index.SaveDir(); err != nil {
		return fmt.Errorf("'%s' could not be saved in the local storage: %s", opts.IndexRepoName, err)
	}

	c.logger.Infof("successfully created index: '%s'\n", opts.IndexRepoName)
	if !opts.Publish {
		return nil
	}

	if err = index.Push(ops...); err != nil {
		return err
	}

	c.logger.Infof("successfully pushed '%s' to registry \n", opts.IndexRepoName)
	return nil
}

func parseOptsToIndexOptions(opts CreateManifestOptions) (idxOpts []imgutil.IndexOption) {
	var format types.MediaType
	switch opts.Format {
	case "oci":
		format = types.OCIImageIndex
	default:
		format = types.DockerManifestList
	}
	if opts.Insecure {
		return []imgutil.IndexOption{
			imgutil.WithMediaType(format),
			imgutil.WithInsecure(),
		}
	}
	return []imgutil.IndexOption{
		imgutil.WithMediaType(format),
	}
}
