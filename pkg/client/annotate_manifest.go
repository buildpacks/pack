package client

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/pkg/image"
)

type ManifestAnnotateOptions struct {
	// Image index we want to update
	IndexRepoName string

	// Name of image we wish to update into the image index
	RepoName string

	// 'os' of the image we wish to update in the image index
	OS string

	// 'architecture' of the image we wish to update in the image index
	OSArch string

	// 'os variant' of the image we wish to update in the image index
	OSVariant string

	// 'annotations' of the image we wish to update in the image index
	Annotations map[string]string
}

// AnnotateManifest implements commands.PackClient.
func (c *Client) AnnotateManifest(ctx context.Context, opts ManifestAnnotateOptions) error {
	idx, err := c.indexFactory.LoadIndex(opts.IndexRepoName)
	if err != nil {
		return err
	}

	imageRef, err := name.ParseReference(opts.RepoName, name.WeakValidation)
	if err != nil {
		return fmt.Errorf("'%s' is not a valid manifest reference: %s", opts.RepoName, err)
	}

	imageToAnnotate, err := c.imageFetcher.Fetch(ctx, imageRef.Name(), image.FetchOptions{Daemon: false})
	if err != nil {
		return err
	}

	hash, err := imageToAnnotate.Identifier()
	if err != nil {
		return err
	}

	digest, err := name.NewDigest(hash.String())
	if err != nil {
		return err
	}

	if opts.OS != "" {
		if err = idx.SetOS(digest, opts.OS); err != nil {
			return fmt.Errorf("'%s' setting the 'os': %s", opts.RepoName, err)
		}
	}
	if opts.OSArch != "" {
		if err = idx.SetArchitecture(digest, opts.OSArch); err != nil {
			return fmt.Errorf("'%s' setting the 'arch': %s", opts.RepoName, err)
		}
	}
	if opts.OSVariant != "" {
		if err = idx.SetVariant(digest, opts.OSVariant); err != nil {
			return fmt.Errorf("'%s' setting the 'os variant': %s", opts.RepoName, err)
		}
	}
	if len(opts.Annotations) != 0 {
		if err = idx.SetAnnotations(digest, opts.Annotations); err != nil {
			return fmt.Errorf("'%s' updating the 'annotations': %s", opts.RepoName, err)
		}
	}

	if err = idx.SaveDir(); err != nil {
		return fmt.Errorf("'%s' could not be saved in the local storage: %s", opts.RepoName, err)
	}

	c.logger.Infof("successfully annotated image '%s' in index '%s'\n", opts.RepoName, opts.IndexRepoName)
	return nil
}
