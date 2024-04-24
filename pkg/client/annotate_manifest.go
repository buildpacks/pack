package client

import (
	"context"
	"fmt"

	ggcrName "github.com/google/go-containerregistry/pkg/name"
)

type ManifestAnnotateOptions struct {
	OS, OSVersion, OSArch, OSVariant string
	OSFeatures, Features, URLs       []string
	Annotations                      map[string]string
}

// AnnotateManifest implements commands.PackClient.
func (c *Client) AnnotateManifest(ctx context.Context, name string, image string, opts ManifestAnnotateOptions) error {
	idx, err := c.indexFactory.LoadIndex(name)
	if err != nil {
		return err
	}

	digest, err := ggcrName.NewDigest(image, ggcrName.Insecure, ggcrName.WeakValidation)
	if err != nil {
		return err
	}

	if opts.OS != "" {
		if err := idx.SetOS(digest, opts.OS); err != nil {
			return err
		}
	}
	if opts.OSVersion != "" {
		if err := idx.SetOSVersion(digest, opts.OSVersion); err != nil {
			return err
		}
	}
	if len(opts.OSFeatures) != 0 {
		if err := idx.SetOSFeatures(digest, opts.OSFeatures); err != nil {
			return err
		}
	}
	if opts.OSArch != "" {
		if err := idx.SetArchitecture(digest, opts.OSArch); err != nil {
			return err
		}
	}
	if opts.OSVariant != "" {
		if err := idx.SetVariant(digest, opts.OSVariant); err != nil {
			return err
		}
	}
	if len(opts.Features) != 0 {
		if err := idx.SetFeatures(digest, opts.Features); err != nil {
			return err
		}
	}
	if len(opts.OSFeatures) != 0 {
		if err := idx.SetOSFeatures(digest, opts.OSFeatures); err != nil {
			return err
		}
	}
	if len(opts.URLs) != 0 {
		if err := idx.SetURLs(digest, opts.URLs); err != nil {
			return err
		}
	}
	if len(opts.Annotations) != 0 {
		if err := idx.SetAnnotations(digest, opts.Annotations); err != nil {
			return err
		}
	}

	if err = idx.Save(); err != nil {
		return err
	}

	fmt.Printf("successfully annotated image '%s' in index '%s'\n", image, name)
	return nil
}
