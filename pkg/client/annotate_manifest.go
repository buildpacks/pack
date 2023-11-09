package client

import (
	"context"
	"fmt"
	"strings"
)

type ManifestAnnotateOptions struct {
	OS, OSVersion, OSArch, OSVariant  string
	OSFeatures, Annotations, Features map[string]string
}

// AnnotateManifest implements commands.PackClient.
func (c *Client) AnnotateManifest(ctx context.Context, name string, image string, opts ManifestAnnotateOptions) error {
	manifestList, err := c.runtime.LookupImageIndex(name)
	if err != nil {
		return err
	}

	digest, err := c.runtime.ParseDigest(image)
	if err != nil {
		ref, _, err := c.imageFactory.FindImage(image)
		if err != nil {
			return err
		}
		digest, err = c.runtime.ParseDigest(ref.Name())
		if err != nil {
			return err
		}
	}

	if opts.OS != "" {
		if err := manifestList.Index.SetOS(digest, opts.OS); err != nil {
			return err
		}
	}
	if opts.OSVersion != "" {
		if err := manifestList.Index.SetOSVersion(digest, opts.OSVersion); err != nil {
			return err
		}
	}
	if len(opts.OSFeatures) != 0 {
		if err := manifestList.Index.SetOSFeatures(digest, opts.OSFeatures); err != nil {
			return err
		}
	}
	if opts.OSArch != "" {
		if err := manifestList.Index.SetArchitecture(digest, opts.OSArch); err != nil {
			return err
		}
	}
	if opts.OSVariant != "" {
		if err := manifestList.Index.SetVariant(digest, opts.OSVariant); err != nil {
			return err
		}
	}
	if len(opts.Features) != 0 {
		if err := manifestList.Index.SetFeatures(digest, opts.Features); err != nil {
			return err
		}
	}
	if len(opts.Annotations) != 0 {
		annotations := make(map[string]string)
		for _, annotationSpec := range opts.Annotations {
			spec := strings.SplitN(annotationSpec, "=", 2)
			if len(spec) != 2 {
				return fmt.Errorf("no value given for annotation %q", spec[0])
			}
			annotations[spec[0]] = spec[1]
		}
		if err := manifestList.Index.SetAnnotations(&digest, annotations); err != nil {
			return err
		}
	}

	updatedListID, err := manifestList.Index.Save(name, nil, "")
	if err == nil {
		fmt.Printf("%s: %s\n", updatedListID, digest.String())
	}

	return nil
}