package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	packErrors "github.com/buildpacks/pack/pkg/errors"
)

type InspectManifestOptions struct {
}

// InspectManifest implements commands.PackClient.
func (c *Client) InspectManifest(ctx context.Context, name string, opts InspectManifestOptions) error {
	printManifest := func(manifest []byte) error {
		var b bytes.Buffer
		err := json.Indent(&b, manifest, "", "    ")
		if err != nil {
			return fmt.Errorf("rendering manifest for display: %w", err)
		}

		fmt.Printf("%s\n", b.String())
		return nil
	}

	// Before doing a remote lookup, attempt to resolve the manifest list
	// locally.
	manifestList, err := c.runtime.LookupImageIndex(name)
	if err == nil {
		schema2List, err := manifestList.Index.Inspect()
		if err != nil {
			rawSchema2List, err := json.Marshal(schema2List)
			if err != nil {
				return err
			}

			return printManifest(rawSchema2List)
		}
		if !errors.Is(err, packErrors.ErrIndexUnknown) && !errors.Is(err, packErrors.ErrNotAddManifestList) {
			return err
		}

		_, err = c.runtime.ParseReference(name)
		if err != nil {
			fmt.Printf("error parsing reference to image %q: %v", name, err)
		}

		index, err := c.indexFactory.FetchIndex(name)

		if err != nil {
			return err
		}

		return printManifest(index)
	}

	return fmt.Errorf("unable to locate manifest list locally or at registry")
}
