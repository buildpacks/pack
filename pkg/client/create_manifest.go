package client

import (
	"context"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
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

	mediaType := imgutil.DockerTypes

	idx, err := remote.NewIndex(
		opts.ManifestName,
		authn.DefaultKeychain,
		remote.WithIndexMediaTypes(mediaType),
		remote.WithPath(opts.LayoutDir)) // This will return an empty index
	if err != nil {
		panic(err)
	}

	// When the publish flag is used all the manifests MUST have os/arch defined otherwise an error must be thrown
	// The format flag will be ignored if it is not used in conjunction with the publish flag

	// Add every manifest to image index
	for _, j := range opts.Manifests {
		err := idx.Add(j)
		if err != nil {
			panic(err)
		}
	}

	// Store layout in local storage
	idx.Save()

	return nil

}
