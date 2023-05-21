package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
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
		return err
	}

	idx, err := local.NewIndex(opts.Index, opts.Path, local.WithManifest(indexManifest))
	if err != nil {
		return err
	}

	// Append manifest to local index
	err = idx.Add(opts.Manifest)
	if err != nil {
		return err
	}

	// Store index in local storage
	err = idx.Save()
	if err != nil {
		return err
	}

	return nil
}
