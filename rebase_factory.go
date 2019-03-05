package pack

import (
	"encoding/json"
	"github.com/buildpack/pack/logging"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/config"
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
	RunImage string
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

	appImage, err := newImage(flags.RepoName)
	if err != nil {
		return RebaseConfig{}, err
	}

	//todo if no -rrun-image and no label run image tell the user something
	//todo: Check  if run image flag has been passed and set that as the run image
	runImageName, err := appImage.Label("io.buildpacks.run-image") // TODO : const the label name
	if err != nil {
		return RebaseConfig{}, err
	}

	if runImageName == "" {
		runImageName = flags.RunImage
	}

	baseImage, err := newImage(runImageName)
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
