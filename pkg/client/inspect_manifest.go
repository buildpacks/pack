package client

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"
)

// InspectManifest implements commands.PackClient.
func (c *Client) InspectManifest(indexRepoName string) error {
	var (
		index    imgutil.ImageIndex
		indexStr string
		err      error
	)

	index, err = c.indexFactory.FindIndex(indexRepoName)
	if err != nil {
		return err
	}

	if indexStr, err = index.Inspect(); err != nil {
		return errors.Wrapf(err, "'%s' printing the index", indexRepoName)
	}

	c.logger.Info(indexStr)
	return nil
}
