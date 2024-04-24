package client

import (
	"context"
	"fmt"

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
func (c *Client) AddManifest(ctx context.Context, ii string, image string, opts ManifestAddOptions) (err error) {
	idx, err := c.indexFactory.LoadIndex(ii)
	if err != nil {
		return err
	}

	var ops = make([]imgutil.IndexAddOption, 0)
	if opts.All {
		ops = append(ops, imgutil.WithAll(opts.All))
	}

	if opts.OS != "" {
		ops = append(ops, imgutil.WithOS(opts.OS))
	}

	if opts.OSArch != "" {
		ops = append(ops, imgutil.WithArchitecture(opts.OSArch))
	}

	if opts.OSVariant != "" {
		ops = append(ops, imgutil.WithVariant(opts.OSVariant))
	}

	if opts.OSVersion != "" {
		ops = append(ops, imgutil.WithOSVersion(opts.OSVersion))
	}

	if len(opts.Features) != 0 {
		ops = append(ops, imgutil.WithFeatures(opts.Features))
	}

	if len(opts.OSFeatures) != 0 {
		ops = append(ops, imgutil.WithOSFeatures(opts.OSFeatures))
	}

	if len(opts.Annotations) != 0 {
		ops = append(ops, imgutil.WithAnnotations(opts.Annotations))
	}

	ref, err := name.ParseReference(image, name.Insecure, name.WeakValidation)
	if err != nil {
		return err
	}

	if err = idx.Add(ref, ops...); err != nil {
		return err
	}

	if err = idx.Save(); err != nil {
		return err
	}

	fmt.Printf("successfully added to index: '%s'\n", image)
	return nil
}
