package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
	"github.com/pkg/errors"
)

type AddManifestOptions struct {
	Index    string
	Path     string
	Manifest string
	All      bool
}

func (c *Client) AddManifest(ctx context.Context, opts AddManifestOptions) error {
	indexManifest, err := local.GetIndexManifest(opts.Index, opts.Path)
	if err != nil {
		return errors.Wrapf(err, "Get local index manifest '%s' from path '%s'", opts.Index, opts.Path)
	}

	idx, err := local.NewIndex(opts.Index, opts.Path, local.WithManifest(indexManifest))
	if err != nil {
		return errors.Wrapf(err, "Create local index from '%s' local index manifest", opts.Index)
	}

	// Append manifest to local index
	err = idx.Add(opts.Manifest)
	if err != nil {
		return errors.Wrapf(err, "Appending '%s' manifest to index '%s'", opts.Manifest, opts.Index)
	}

	// Store index in local storage
	err = idx.Save()
	if err != nil {
		return errors.Wrapf(err, "Save local index '%s' at '%s' path", opts.Index, opts.Path)
	}

	return nil
}
