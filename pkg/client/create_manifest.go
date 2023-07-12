package client

import (
	"context"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"
)

type CreateManifestOptions struct {
	// Name of the ManifestList.
	ManifestName string

	// List of Images
	Manifests []string

	// Manifest list type (oci or v2s2) to use when pushing the list (default is v2s2)
	MediaType imgutil.MediaTypes

	// Skip creating index locally, directly publish to a registry.
	// Requires ManifestName to be a valid registry location.
	Publish bool

	// Defines the registry to publish the manifest list.
	Registry string

	// Directory to store OCI layout
	ManifestDir string
}

func (c *Client) CreateManifest(ctx context.Context, opts CreateManifestOptions) error {
	indexCreator := c.indexFactory
	idx, err := indexCreator.NewIndex(opts)
	if err != nil {
		if opts.Publish {
			return errors.Wrapf(err, "Failed to create remote index '%s'", opts.ManifestName)
		} else {
			return errors.Wrapf(err, "Failed to create local index '%s'", opts.ManifestName)
		}
	}

	// Add every manifest to image index
	for _, j := range opts.Manifests {
		err := idx.Add(j)
		if err != nil {
			return errors.Wrapf(err, "Appending manifest to index '%s'", opts.ManifestName)
		}
	}

	// Store index
	err = idx.Save()
	if err != nil {
		if opts.Publish {
			return errors.Wrapf(err, "Storing index '%s' in registry.", opts.ManifestName)
		} else {
			return errors.Wrapf(err, "Save local index '%s' at '%s' path", opts.ManifestName, opts.ManifestDir)
		}
	}

	return nil
}
