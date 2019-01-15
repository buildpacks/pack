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
	Image            string
	LocalRunImages   []string
	DefaultRunImages []string
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
	defaultRunImages, err := b.getDefaultRunImages(builderImage)
	if err != nil {
		return Builder{}, err
	}

	builderName := builderImage.Name()
	return Builder{
		Image:            builderName,
		LocalRunImages:   b.getLocalRunImages(builderName),
		DefaultRunImages: defaultRunImages,
	}, nil
}

func (b *BuilderInspect) getLocalRunImages(builderName string) []string {
	if builderConfig := b.Config.GetBuilder(builderName); builderConfig != nil {
		return builderConfig.RunImages
	}
	return nil
}

func (b *BuilderInspect) getDefaultRunImages(builderImage image.Image) ([]string, error) {
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
	return metadata.RunImages, nil
}
