package pack

import (
	"context"
	"encoding/json"

	"github.com/buildpack/lifecycle/metadata"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"

	"github.com/buildpack/pack/style"
)

type RebaseOptions struct {
	RepoName          string
	Publish           bool
	SkipPull          bool
	RunImage          string
	AdditionalMirrors map[string][]string
}

func (c *Client) Rebase(ctx context.Context, opts RebaseOptions) error {
	imageRef, err := c.parseTagReference(opts.RepoName)
	if err != nil {
		return errors.Wrapf(err, "invalid image name '%s'", opts.RepoName)
	}

	appImage, err := c.imageFetcher.Fetch(ctx, opts.RepoName, !opts.Publish, !opts.SkipPull)
	if err != nil {
		return err
	}

	md, err := metadata.GetAppMetadata(appImage)
	if err != nil {
		return err
	}

	runImageName := c.resolveRunImage(
		opts.RunImage,
		imageRef.Context().RegistryStr(),
		builder.StackMetadata{
			RunImage: builder.RunImageMetadata{
				Image:   md.Stack.RunImage.Image,
				Mirrors: md.Stack.RunImage.Mirrors,
			},
		},
		opts.AdditionalMirrors)

	if runImageName == "" {
		return errors.New("run image must be specified")
	}

	baseImage, err := c.imageFetcher.Fetch(ctx, runImageName, !opts.Publish, !opts.SkipPull)
	if err != nil {
		return err
	}

	c.logger.Infof("Rebasing %s on run image %s", style.Symbol(appImage.Name()), style.Symbol(baseImage.Name()))
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
	if err != nil {
		return err
	}

	if err := appImage.SetLabel(metadata.AppMetadataLabel, string(newLabel)); err != nil {
		return err
	}

	sha, err := appImage.Save()
	if err != nil {
		return err
	}
	c.logger.Infof("New sha: %s", style.Symbol(sha))
	return nil
}
