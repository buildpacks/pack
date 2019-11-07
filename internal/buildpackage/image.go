package buildpackage

import (
	"github.com/buildpack/imgutil"

	image2 "github.com/buildpack/pack/internal/image"
)

const metadataLabel = "io.buildpacks.buildpackage.metadata"

type Image interface {
	imgutil.Image
	Metadata() Metadata
	SetMetadata(Metadata) error

	// TODO: Should buildpackages have this label set?
	//BuildpackLayers() BuildpackLayers
	//SetBuildpackLayers(layers BuildpackLayers) error
}

// TODO: Test this
type image struct {
	imgutil.Image
	metadata Metadata
}

func NewPackageImage(raw imgutil.Image) (Image, error) {
	var metadata Metadata
	if _, err := image2.UnmarshalLabel(raw, metadataLabel, &metadata); err != nil {
		return nil, err
	}

	return &image{
		Image:    raw,
		metadata: metadata,
	}, nil
}

func (i *image) Metadata() Metadata {
	return i.metadata
}

func (i *image) SetMetadata(metadata Metadata) error {
	if err := image2.MarshalToLabel(i, metadataLabel, metadata); err != nil {
		return err
	}
	i.metadata = metadata
	return nil
}
