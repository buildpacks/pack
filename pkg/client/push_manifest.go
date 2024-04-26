package client

import (
	"github.com/pkg/errors"
)

type PushManifestOptions struct {
	// Image index we want to update
	IndexRepoName string

	// Index media-type
	Format string

	// true if we want to publish to an insecure registry
	Insecure bool

	// true if we want the index to be deleted from local storage after pushing it
	Purge bool
}

// PushManifest implements commands.PackClient.
func (c *Client) PushManifest(opts PushManifestOptions) (err error) {
	idx, err := c.indexFactory.LoadIndex(opts.IndexRepoName)
	if err != nil {
		return
	}

	// TODO pass through the options
	if err = idx.Push(); err != nil {
		return errors.Wrapf(err, "pushing index '%s'", opts.IndexRepoName)
	}

	if !opts.Purge {
		c.logger.Infof("successfully pushed index: '%s'\n", opts.IndexRepoName)
		return nil
	}

	return idx.DeleteDir()
}

/*
func parseFalgsForImgUtil(opts PushManifestOptions) (idxOptions []imgutil.IndexOption) {
	idxOptions = append(idxOptions, imgutil.WithInsecure())

	if opts.Purge {
		idxOptions = append(idxOptions, imgutil.WithPurge(true))
	}

	switch opts.Format {
	case "oci":
		return append(idxOptions, imgutil.WithMediaType(types.OCIImageIndex))
	case "v2s2":
		return append(idxOptions, imgutil.WithMediaType(types.DockerManifestList))
	default:
		return idxOptions
	}
}
*/
