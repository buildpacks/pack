package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/name"
)

type ManifestAddOptions struct {
	OS, OSVersion, OSArch, OSVariant string
	OSFeatures, Features             []string
	Annotations                      map[string]string
	All                              bool
}

// AddManifest implements commands.PackClient.
func (c *Client) AddManifest(ctx context.Context, index string, image string, opts ManifestAddOptions) (err error) {
	if _, err = name.ParseReference(index); err != nil {
		return
	}

	ref, err := name.ParseReference(image)
	if err != nil {
		return err
	}

	imgIndex, err := c.indexFactory.LoadIndex(index)
	if err != nil {
		return fmt.Errorf("Error while trying to find image on local storage: %v", image)
	}

	err = imgIndex.Add(ref, imgutil.WithAll(opts.All))
	if err != nil {
		return fmt.Errorf("Error while trying to add on manifest list: %v", err)
	}

	if opts.OS != "" {
		if err := imgIndex.SetOS(ref.(name.Digest), opts.OS); err != nil {
			return err
		}
	}

	if opts.OSArch != "" {
		if err := imgIndex.SetArchitecture(ref.(name.Digest), opts.OSArch); err != nil {
			return err
		}
	}

	if opts.OSVariant != "" {
		if err := imgIndex.SetVariant(ref.(name.Digest), opts.OSVariant); err != nil {
			return err
		}
	}

	if opts.OSVersion != "" {
		if err := imgIndex.SetOSVersion(ref.(name.Digest), opts.OSVersion); err != nil {
			return err
		}
	}

	if len(opts.Features) != 0 {
		if err := imgIndex.SetFeatures(ref.(name.Digest), opts.Features); err != nil {
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
		if err := imgIndex.SetAnnotations(ref.(name.Digest), annotations); err != nil {
			return err
		}
	}

	err = imgIndex.Save()
	if err == nil {
		fmt.Printf("'%s' successfully added to index: '%s'", image, index)
	}

	return err
}
