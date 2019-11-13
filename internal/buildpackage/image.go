package buildpackage

import (
	"github.com/buildpack/imgutil"

	"github.com/buildpack/pack/internal/image"
)

const metadataLabel = "io.buildpacks.buildpackage.metadata"

type Image interface {
	imgutil.Image
	Metadata() Metadata
	SetMetadata(Metadata) error
}

type packageImage struct {
	imgutil.Image
	metadata Metadata
}

func NewImage(img imgutil.Image) (Image, error) {
	var metadata Metadata
	if _, err := image.UnmarshalLabel(img, metadataLabel, &metadata); err != nil {
		return nil, err
	}

	return &packageImage{
		Image:    img,
		// metadata: metadata,
	}, nil
}

func (i *packageImage) Metadata() Metadata {
	// return i.metadata
}

// func (i *packageImage) SetMetadata(metadata Metadata) error {
// 	if err := image.MarshalToLabel(i, metadataLabel, metadata); err != nil {
// 		return err
// 	}
// 	i.metadata = metadata
// 	return nil
// }
