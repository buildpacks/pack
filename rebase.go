package pack

import (
	"context"
	"encoding/json"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/lifecycle/metadata"
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
	appImage, err := c.imageFetcher.Fetch(ctx, opts.RepoName, !opts.Publish, !opts.SkipPull)
	if err != nil {
		return err
	}

	runImageName, err := c.getRunImageName(ctx, opts, appImage)
	if err != nil {
		return err
	}

	baseImage, err := c.imageFetcher.Fetch(ctx, runImageName, !opts.Publish, !opts.SkipPull)
	if err != nil {
		return err
	}

	label, err := appImage.Label(metadata.AppMetadataLabel)
	if err != nil {
		return err
	}
	var md metadata.AppImageMetadata
	if err := json.Unmarshal([]byte(label), &md); err != nil {
		return err
	}
	c.logger.Info("Rebasing %s on run image %s", style.Symbol(appImage.Name()), style.Symbol(baseImage.Name()))
	if err := appImage.Rebase(md.RunImage.TopLayer, baseImage); err != nil {
		return err
	}

	md.RunImage.SHA, err = baseImage.Digest()
	if err != nil {
		return err
	}

	md.RunImage.TopLayer, err = baseImage.TopLayer()
	if err != nil {
		return err
	}

	newLabel, err := json.Marshal(md)
	if err := appImage.SetLabel(metadata.AppMetadataLabel, string(newLabel)); err != nil {
		return err
	}

	sha, err := appImage.Save()
	if err != nil {
		return err
	}
	c.logger.Info("New sha: %s", style.Symbol(sha))
	return nil
}

func (c *Client) getRunImageName(ctx context.Context, opts RebaseOptions, appImage imgutil.Image) (string, error) {
	var runImageName string
	if opts.RunImage != "" {
		runImageName = opts.RunImage
	} else {
		contents, err := appImage.Label(metadata.AppMetadataLabel)
		if err != nil {
			return "", err
		}

		var appImageMetadata metadata.AppImageMetadata
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
