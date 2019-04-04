package pack

import (
	"context"
	"encoding/json"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/style"
)

type RebaseOptions struct {
	RepoName string
	Publish  bool
	SkipPull bool
	RunImage string
}

func (c *Client) Rebase(ctx context.Context, opts RebaseOptions) error {
	fetchImage := c.imageFetchFn(ctx, opts)

	appImage, err := fetchImage(opts.RepoName)
	if err != nil {
		return err
	}

	runImageName, err := c.getRunImageName(ctx, opts, appImage)
	if err != nil {
		return err
	}

	baseImage, err := fetchImage(runImageName)
	if err != nil {
		return err
	}

	label, err := appImage.Label(lifecycle.MetadataLabel)
	if err != nil {
		return err
	}
	var metadata lifecycle.AppImageMetadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return err
	}
	c.logger.Info("Rebasing %s on run image %s", style.Symbol(appImage.Name()), style.Symbol(baseImage.Name()))
	if err := appImage.Rebase(metadata.RunImage.TopLayer, baseImage); err != nil {
		return err
	}

	metadata.RunImage.SHA, err = baseImage.Digest()
	if err != nil {
		return err
	}
	metadata.RunImage.TopLayer, err = baseImage.TopLayer()
	if err != nil {
		return err
	}
	newLabel, err := json.Marshal(metadata)
	if err := appImage.SetLabel(lifecycle.MetadataLabel, string(newLabel)); err != nil {
		return err
	}

	sha, err := appImage.Save()
	if err != nil {
		return err
	}
	c.logger.Info("New sha: %s", style.Symbol(sha))
	return nil
}

func (c *Client) imageFetchFn(ctx context.Context, opts RebaseOptions) func(string) (image.Image, error) {
	var newImageFn func(string) (image.Image, error)
	if opts.Publish {
		newImageFn = c.fetcher.FetchRemoteImage
	} else {
		newImageFn = func(name string) (image.Image, error) {
			if opts.SkipPull {
				return c.fetcher.FetchLocalImage(name)

			} else {
				return c.fetcher.FetchUpdatedLocalImage(ctx, name, c.logger.RawVerboseWriter())
			}
		}
	}
	return newImageFn
}

func (c *Client) getRunImageName(ctx context.Context, opts RebaseOptions, appImage image.Image) (string, error) {
	var runImageName string
	if opts.RunImage != "" {
		runImageName = opts.RunImage
	} else {
		contents, err := appImage.Label(lifecycle.MetadataLabel)
		if err != nil {
			return "", err
		}

		var appImageMetadata lifecycle.AppImageMetadata
		if err := json.Unmarshal([]byte(contents), &appImageMetadata); err != nil {
			return "", err
		}

		registry, err := config.Registry(opts.RepoName)
		if err != nil {
			return "", errors.Wrapf(err, "parsing registry from reference '%s'", opts.RepoName)
		}

		var mirrors []string
		if localRunImage := c.config.GetRunImage(appImageMetadata.Stack.RunImage.Image); localRunImage != nil {
			mirrors = localRunImage.Mirrors
		}
		mirrors = append(mirrors, appImageMetadata.Stack.RunImage.Image)
		mirrors = append(mirrors, appImageMetadata.Stack.RunImage.Mirrors...)
		runImageName, err = config.ImageByRegistry(registry, mirrors)
		if err != nil {
			return "", errors.Wrapf(err, "find image by registry")
		}
	}

	if runImageName == "" {
		return "", errors.New("run image must be specified")
	}

	return runImageName, nil
}
