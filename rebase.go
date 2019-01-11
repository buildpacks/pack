package pack

import (
	"encoding/json"
	"fmt"
	"github.com/buildpack/pack/logging"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/style"
)

type RebaseConfig struct {
	Image        image.Image
	NewBaseImage image.Image
}

type RebaseFactory struct {
	Logger       *logging.Logger
	Config       *config.Config
	ImageFactory ImageFactory
}

type RebaseFlags struct {
	RepoName string
	Publish  bool
	NoPull   bool
}

func (f *RebaseFactory) RebaseConfigFromFlags(flags RebaseFlags) (RebaseConfig, error) {
	var newImage func(string) (image.Image, error)
	if flags.Publish {
		newImage = f.ImageFactory.NewRemote
	} else {
		newImage = func(name string) (image.Image, error) {
			return f.ImageFactory.NewLocal(name, !flags.NoPull)
		}
	}

	image, err := newImage(flags.RepoName)
	if err != nil {
		return RebaseConfig{}, err
	}

	stackID, err := image.Label(StackLabel)
	if err != nil {
		return RebaseConfig{}, err
	}

	baseImageName, err := f.runImageName(stackID, flags.RepoName)
	if err != nil {
		return RebaseConfig{}, err
	}

	baseImage, err := newImage(baseImageName)
	if err != nil {
		return RebaseConfig{}, err
	}
	return RebaseConfig{
		Image:        image,
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

	_, err = cfg.Image.Save()
	if err != nil {
		return err
	}
	return nil
}

func (f *RebaseFactory) runImageName(stackID, repoName string) (string, error) {
	stack, err := f.Config.GetStack(stackID)
	if err != nil {
		return "", err
	}
	if len(stack.RunImages) == 0 {
		return "", fmt.Errorf("invalid stack: stack %s requies at least one run image", style.Symbol(stack.ID))
	}
	registry, err := config.Registry(repoName)
	if err != nil {
		return "", err
	}
	return config.ImageByRegistry(registry, stack.RunImages)
}
