package pack

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/style"
)

type BuilderInspect struct {
	Config *config.Config
}

type Builder struct {
	Image                string
	RunImage             string
	LocalRunImageMirrors []string
	RunImageMirrors      []string
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
	defaultRunImage, err := b.getRunImageMirrors(builderImage)
	if err != nil {
		return Builder{}, err
	}

	builderName := builderImage.Name()
	return Builder{
		Image:                builderName,
		RunImage:             defaultRunImage.Image,
		LocalRunImageMirrors: b.getLocalRunImageMirrors(defaultRunImage.Image),
		RunImageMirrors:      defaultRunImage.Mirrors,
	}, nil
}

func (b *BuilderInspect) getLocalRunImageMirrors(imageName string) []string {
	if builderConfig := b.Config.GetRunImage(imageName); builderConfig != nil {
		return builderConfig.Mirrors
	}
	return nil
}

func (b *BuilderInspect) getRunImageMirrors(builderImage image.Image) (*BuilderRunImageMetadata, error) {
	var metadata BuilderImageMetadata

	label, err := builderImage.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find run images for builder %s", style.Symbol(builderImage.Name()))
	}
	if label == "" {
		return nil, fmt.Errorf("invalid builder image %s: missing required label %s -- try recreating builder", style.Symbol(builderImage.Name()), style.Symbol(MetadataLabel))
	}
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to parse run images for builder %s", style.Symbol(builderImage.Name()))
	}

	return &metadata.RunImage, nil
}
