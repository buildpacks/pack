package client

import (
	"context"

	"github.com/buildpacks/imgutil"
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
	LayoutDir string
}

func (c *Client) CreateManifest(ctx context.Context, opts CreateManifestOptions) error {

	indexCreator := c.indexFactory
	idx, err := indexCreator.NewIndex(opts)
	if err != nil {
		panic(err)
	}

	// Add every manifest to image index
	for _, j := range opts.Manifests {
		err := idx.Add(j)
		if err != nil {
			panic(err)
		}
	}

	// Store index
	err = idx.Save()
	if err != nil {
		panic(err)
	}

	return nil

}
