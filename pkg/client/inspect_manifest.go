package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/buildpacks/imgutil/local"
	"github.com/pkg/errors"
)

type InspectManifestOptions struct {
	Index string
	Path  string
}

func (c *Client) InspectManifest(ctx context.Context, opts InspectManifestOptions) error {
	indexManifest, err := local.GetIndexManifest(opts.Index, opts.Path)
	if err == nil {
		data, err := json.MarshalIndent(indexManifest, "", "    ")
		if err != nil {
			return errors.Wrapf(err, "Marshal the '%s' manifest information", opts.Index)
		}

		c.logger.Infof("%s\n", string(data))
		return nil
	}

	return fmt.Errorf("Index %s not found in local storage", opts.Index)
}
