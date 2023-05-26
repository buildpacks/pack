package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/pkg/errors"
)

type PushManifestOptions struct {
	Index string
	Path  string
}

func (c *Client) PushManifest(ctx context.Context, opts PushManifestOptions) error {
	indexManifest, err := local.GetIndexManifest(opts.Index, opts.Path)
	if err != nil {
		return errors.Wrapf(err, "Get local index manifest '%s' from path '%s'", opts.Index, opts.Path)
	}

	idx, err := remote.NewIndex(opts.Index, c.keychain, remote.WithManifest(indexManifest))
	if err != nil {
		return errors.Wrapf(err, "Create remote index from '%s' local index manifest", opts.Index)
	}

	// Store index
	err = idx.Save()
	if err != nil {
		return errors.Wrapf(err, "Storing index '%s' in registry. Check if all the referenced manifests are in the same repository in registry", opts.Index)
	}

	return nil
}
