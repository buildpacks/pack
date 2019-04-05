package pack

import (
	"context"
	"encoding/json"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

type RebaseConfig struct {
	Image        image.Image
	NewBaseImage image.Image
}

type RebaseFactory struct {
	Logger  *logging.Logger
	Config  *config.Config
	Fetcher Fetcher
}

type RebaseFlags struct {
	RepoName string
	Publish  bool
	NoPull   bool
	RunImage string
}

func (f *RebaseFactory) RebaseConfigFromFlags(ctx context.Context, flags RebaseFlags) (RebaseConfig, error) {
	var newImageFn func(string) (image.Image, error)
	if flags.Publish {
		newImageFn = f.Fetcher.FetchRemoteImage
	} else {
		newImageFn = func(name string) (image.Image, error) {
			if !flags.NoPull {
				return f.Fetcher.FetchUpdatedLocalImage(ctx, name, f.Logger.RawVerboseWriter())

			} else {
				return f.Fetcher.FetchLocalImage(name)

			}
		}
	}

	appImage, err := newImageFn(flags.RepoName)
	if err != nil {
		return RebaseConfig{}, err
	}

	var runImageName string
	if flags.RunImage != "" {
		runImageName = flags.RunImage
	} else {
		contents, err := appImage.Label(lifecycle.MetadataLabel)
		if err != nil {
			return RebaseConfig{}, err
		}

		var appImageMetadata lifecycle.AppImageMetadata
		if err := json.Unmarshal([]byte(contents), &appImageMetadata); err != nil {
			return RebaseConfig{}, err
		}

		registry, err := config.Registry(flags.RepoName)
		if err != nil {
			return RebaseConfig{}, errors.Wrapf(err, "parsing registry from reference '%s'", flags.RepoName)
		}

		mirrors := make([]string, 0)
		if localRunImage := f.Config.GetRunImage(appImageMetadata.Stack.RunImage.Image); localRunImage != nil {
			mirrors = append(mirrors, localRunImage.Mirrors...)
		}
		mirrors = append(mirrors, appImageMetadata.Stack.RunImage.Image)
		mirrors = append(mirrors, appImageMetadata.Stack.RunImage.Mirrors...)
		runImageName, err = config.ImageByRegistry(registry, mirrors)
		if err != nil {
			return RebaseConfig{}, errors.Wrapf(err, "find image by registry")
		}
	}

	if runImageName == "" {
		return RebaseConfig{}, errors.New("run image must be specified")
	}

	baseImage, err := newImageFn(runImageName)
	if err != nil {
		return RebaseConfig{}, err
	}

	return RebaseConfig{
		Image:        appImage,
		NewBaseImage: baseImage,
	}, nil
}

func (f *RebaseFactory) Rebase(cfg RebaseConfig) error {
	label, err := cfg.Image.Label("io.buildpacks.lifecycle.metadata")
	if err != nil {
		return err
	}
	var metadata lifecycle.AppImageMetadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return err
	}
	f.Logger.Info("Rebasing %s on run image %s", style.Symbol(cfg.Image.Name()), style.Symbol(cfg.NewBaseImage.Name()))
	if err := cfg.Image.Rebase(metadata.RunImage.TopLayer, cfg.NewBaseImage); err != nil {
		return err
	}

	metadata.RunImage.SHA, err = cfg.NewBaseImage.Digest()
	if err != nil {
		return err
	}
	metadata.RunImage.TopLayer, err = cfg.NewBaseImage.TopLayer()
	if err != nil {
		return err
	}
	newLabel, err := json.Marshal(metadata)
	if err := cfg.Image.SetLabel("io.buildpacks.lifecycle.metadata", string(newLabel)); err != nil {
		return err
	}

	sha, err := cfg.Image.Save()
	if err != nil {
		return err
	}
	f.Logger.Info("New sha: %s", style.Symbol(sha))
	return nil
}
