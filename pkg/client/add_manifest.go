package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

type ManifestAddOptions struct {
	OS, OSVersion, OSArch, OSVariant  string
	OSFeatures, Features []string
	Annotations map[string]string
	All bool
}

// AddManifest implements commands.PackClient.
func (c *Client) AddManifest(ctx context.Context, index string, image string, opts ManifestAddOptions) (indexID string, err error) {
	_, err = name.ParseReference(index)
	if err != nil {
		return
	}

	ref, err := name.ParseReference(image)
	if err != nil {
		return
	}

	imgIndex, err := c.indexFactory.FindIndex(index)
	if err != nil {
		return indexID, fmt.Errorf("Error while trying to find image on local storage: %v", image)
	}

	digest, err := imgIndex.Add(ctx, ref, opts.All)
	if err != nil {
		return indexID, fmt.Errorf("Error while trying to add on manifest list: %v", err)
	}

	if opts.OS != "" {
		if _, err := imgIndex.SetOS(digest, opts.OS); err != nil {
			return indexID, err
		}
	}

	if opts.OSArch != "" {
		if _, err := imgIndex.SetArchitecture(digest, opts.OSArch); err != nil {
			return indexID, err
		}
	}

	if opts.OSVariant != "" {
		if _, err := imgIndex.SetVariant(digest, opts.OSVariant); err != nil {
			return indexID, err
		}
	}

	if opts.OSVersion != "" {
		if _, err := imgIndex.SetOSVersion(digest, opts.OSVersion); err != nil {
			return indexID, err
		}
	}

	if len(opts.Features) != 0 {
		if _, err := imgIndex.SetFeatures(digest, opts.Features); err != nil {
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
		if err := imgIndex.SetAnnotations(&digest, annotations); err != nil {
			return err
		}
	}

	indexID, err = imgIndex.Save(index, nil, "")
	if err == nil {
		fmt.Printf("%s: %s\n", indexID, digest.String())
	}

	return indexID, err
}
