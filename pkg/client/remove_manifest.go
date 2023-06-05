package client

import (
	"context"

	"github.com/buildpacks/imgutil/local"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
)

type DeleteManifestOptions struct {
	Index string
	Path  string
}

func (c *Client) DeleteManifest(ctx context.Context, opts DeleteManifestOptions) error {
	indexManifest, err := local.GetIndexManifest(opts.Index, opts.Path)
	if err != nil {
		return errors.Wrapf(err, "Get local index manifest '%s' from path '%s'", opts.Index, opts.Path)
	}

	idx, err := local.NewIndex(opts.Index, opts.Path, local.WithManifest(indexManifest))
	if err != nil {
		return errors.Wrapf(err, "Create local index from '%s' local index manifest", opts.Index)
	}

	err = idx.Delete()
	if err != nil {
		return errors.Wrapf(err, "Failed to remove index '%s' from local storage\n", style.Symbol(opts.Index))
	}

	return nil
}
