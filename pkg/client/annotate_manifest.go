package client

import (
	"context"
	"fmt"
	"strings"

	ggcrName "github.com/google/go-containerregistry/pkg/name"
)

type ManifestAnnotateOptions struct {
	OS, OSVersion, OSArch, OSVariant  string
	OSFeatures, Annotations, Features map[string]string
}

// AnnotateManifest implements commands.PackClient.
func (c *Client) AnnotateManifest(ctx context.Context, name string, image string, opts ManifestAnnotateOptions) error {
	manifestList, err := c.indexFactory.FindIndex(name)
	if err != nil {
		return err
	}

	digest, err := ggcrName.NewDigest(image)
	if err != nil {
		ref, _, err := c.imageFactory.FindImage(image)
		if err != nil {
			return fmt.Errorf("Error while trying to find image on local storage: %v", err.Error())
		}
		digest, err = ggcrName.NewDigest(ref.Name())
		if err != nil {
			return err
		}
	}

	if opts.OS != "" {
		if err := manifestList.SetOS(digest, opts.OS); err != nil {
			return err
		}
	}
	if opts.OSVersion != "" {
		if err := manifestList.SetOSVersion(digest, opts.OSVersion); err != nil {
			return err
		}
	}
	if len(opts.OSFeatures) != 0 {
		if err := manifestList.SetOSFeatures(digest, opts.OSFeatures); err != nil {
			return err
		}
	}
	if opts.OSArch != "" {
		if err := manifestList.SetArchitecture(digest, opts.OSArch); err != nil {
			return err
		}
	}
	if opts.OSVariant != "" {
		if err := manifestList.SetVariant(digest, opts.OSVariant); err != nil {
			return err
		}
	}
	if len(opts.Features) != 0 {
		if err := manifestList.SetFeatures(digest, opts.Features); err != nil {
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
		if err := manifestList.SetAnnotations(&digest, annotations); err != nil {
			return err
		}
	}

	updatedListID, err := manifestList.Save(name, nil, "")
	if err == nil {
		fmt.Printf("%s: %s\n", updatedListID, digest.String())
	}

	return nil
}
