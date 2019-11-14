package buildpackage

import (
	"github.com/buildpack/imgutil"

	"github.com/buildpack/pack/internal/image"
)

type Image interface {
	Name() string
	Metadata() Metadata
}

type packageImage struct {
	name     string
	metadata Metadata
}

func NewImage(img imgutil.Image) (Image, error) {
	var metadata Metadata
	if _, err := image.UnmarshalLabel(img, metadataLabel, &metadata); err != nil {
		return nil, err
	}

	return &packageImage{
		name:     img.Name(),
		metadata: metadata,
	}, nil
}

func (i *packageImage) Name() string {
	return i.name
}

func (i *packageImage) Metadata() Metadata {
	return i.metadata
}
