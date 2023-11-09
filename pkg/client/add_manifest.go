package client

import (
	"context"
	"fmt"
	"strings"
)

type ManifestAddOptions struct {
	ManifestAnnotateOptions
	All bool
}

// AddManifest implements commands.PackClient.
func (c *Client) AddManifest(ctx context.Context, index string, image string, opts ManifestAddOptions) (indexID string, err error) {
	ref, err := c.runtime.ParseReference(image)
	if err != nil {
		return
	}

	imgIndex, err := c.runtime.LookupImageIndex(index)
	if err != nil {
		return
	}

	digest, err := imgIndex.Add(ctx, ref, opts.All)
	if err != nil {
		if ref, _, err = c.imageFactory.FindImage(image); err != nil {
			return indexID, fmt.Errorf("Error while trying to find image on local storage: %v", err)
		}
		digest, err = imgIndex.Add(ctx, ref, opts.All)
		if err != nil {
			return indexID, fmt.Errorf("Error while trying to add on manifest list: %v", err)
		}
	}

	if opts.OS != "" {
		if _, err := imgIndex.Index.SetOS(digest, opts.OS); err != nil {
			return indexID, err
		}
	}

	if opts.OSArch != "" {
		if _, err := imgIndex.Index.SetArchitecture(digest, opts.OSArch); err != nil {
			return indexID, err
		}
	}

	if opts.OSVariant != "" {
		if _, err := imgIndex.Index.SetVariant(digest, opts.OSVariant); err != nil {
			return indexID, err
		}
	}

	if opts.OSVersion != "" {
		if _, err := imgIndex.Index.SetOSVersion(digest, opts.OSVersion); err != nil {
			return indexID, err
		}
	}

	if len(opts.Features) != 0 {
		if _, err := imgIndex.Index.SetFeatures(digest, opts.Features); err != nil {
			return indexID, err
		}
	}

	if len(opts.Annotations) != 0 {
		annotations := make(map[string]string)
		for _, annotationSpec := range opts.Annotations {
			spec := strings.SplitN(annotationSpec, "=", 2)
			if len(spec) != 2 {
				return indexID, fmt.Errorf("no value given for annotation %q", spec[0])
			}
			annotations[spec[0]] = spec[1]
		}
		if err := imgIndex.Index.SetAnnotations(&digest, annotations); err != nil {
			return err
		}
	}

	indexID, err = imgIndex.Index.Save(index, nil, "")
	if err == nil {
		fmt.Printf("%s: %s\n", indexID, digest.String())
	}

	return indexID, err
}