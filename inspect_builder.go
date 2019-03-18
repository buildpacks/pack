package pack

import (
	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/config"
)

type BuilderInspect struct {
	Config *config.Config
}

type Builder struct {
	Image                string
	RunImage             string
	LocalRunImageMirrors []string
	RunImageMirrors      []string
	Buildpacks           []builder.BuildpackMetadata
	Groups               []builder.GroupMetadata
}

func DefaultBuilderInspect() (*BuilderInspect, error) {
	cfg, err := config.NewDefault()
	if err != nil {
		return nil, err
	}

	return &BuilderInspect{
		Config: cfg,
	}, nil
}

func (b *BuilderInspect) Inspect(builderImage image.Image) (Builder, error) {
	builderMetadata, err := builder.GetMetadata(builderImage)
	if err != nil {
		return Builder{}, err
	}

	return Builder{
		Image:                builderImage.Name(),
		RunImage:             builderMetadata.RunImage.Image,
		LocalRunImageMirrors: b.getLocalRunImageMirrors(builderMetadata.RunImage.Image),
		RunImageMirrors:      builderMetadata.RunImage.Mirrors,
		Buildpacks:           builderMetadata.Buildpacks,
		Groups:               builderMetadata.Groups,
	}, nil
}

func (b *BuilderInspect) getLocalRunImageMirrors(imageName string) []string {
	if builderConfig := b.Config.GetRunImage(imageName); builderConfig != nil {
		return builderConfig.Mirrors
	}
	return nil
}