package builder

import (
	"encoding/json"
	"fmt"

	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/style"
)

type Builder struct {
	image  image.Image
	config *config.Config
}

func NewBuilder(img image.Image, cfg *config.Config) *Builder {
	return &Builder{
		image:  img,
		config: cfg,
	}
}

func (b *Builder) GetStack() (string, error) {
	stack, err := b.image.Label("io.buildpacks.stack.id")
	if err != nil {
		return "", errors.Wrapf(err, "failed to find stack label for builder %s", style.Symbol(b.image.Name()))
	}

	if stack == "" {
		return "", fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(b.image.Name()), style.Symbol("io.buildpacks.stack.id"))
	}

	return stack, nil
}

func (b *Builder) GetMetadata() (*Metadata, error) {
	label, err := b.image.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find run images for builder %s", style.Symbol(b.image.Name()))
	}

	if label == "" {
		return nil, fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(b.image.Name()), style.Symbol(MetadataLabel))
	}

	var metadata Metadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to parse metadata for builder %s", style.Symbol(b.image.Name()))
	}

	return &metadata, nil
}

func (b *Builder) GetLocalRunImageMirrors() ([]string, error) {
	metadata, err := b.GetMetadata()
	if err != nil {
		return nil, err
	}
	if runImageConfig := b.config.GetRunImage(metadata.RunImage.Image); runImageConfig != nil {
		return runImageConfig.Mirrors, nil
	}
	return []string{}, nil
}
