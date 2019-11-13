package builder

import (
	"github.com/buildpack/imgutil"

	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/image"
)

type Image interface {
	imgutil.Image
	StackID() string
	CommonMixins() []string
	Order() dist.Order
	SetOrder(order dist.Order) error
	Metadata() Metadata
	SetMetadata(metadata Metadata) error
	BuildpackLayers() BuildpackLayers
	SetBuildpackLayers(bpLayers BuildpackLayers) error
	BuildOnlyMixins() []string
}

//go:generate mockgen -package testmocks -destination testmocks/mock_build_image.go github.com/buildpack/pack/internal/builder BuildImage
type BuildImage interface {
	imgutil.Image
	StackID() string
	CommonMixins() []string
	BuildOnlyMixins() []string
}

// TODO: Test this
type builderImage struct {
	BuildImage
	metadata Metadata
	order    dist.Order
	bpLayers BuildpackLayers
}

func NewImage(img BuildImage) (Image, error) {
	var metadata Metadata
	if _, err := image.UnmarshalLabel(img, metadataLabel, &metadata); err != nil {
		return nil, err
	}

	var order dist.Order
	if ok, err := image.UnmarshalLabel(img, orderLabel, &order); err != nil {
		return nil, err
	} else if !ok {
		order = metadata.Groups.ToOrder()
	}

	bpLayers := BuildpackLayers{}
	if _, err := image.UnmarshalLabel(img, buildpackLayersLabel, &bpLayers); err != nil {
		return nil, err
	}

	return &builderImage{
		BuildImage: img,
		metadata:   metadata,
		order:      order,
		bpLayers:   bpLayers,
	}, nil
}

func (i *builderImage) Metadata() Metadata {
	return i.metadata
}

func (i *builderImage) SetMetadata(metadata Metadata) error {
	if err := image.MarshalToLabel(i, metadataLabel, metadata); err != nil {
		return err
	}
	i.metadata = metadata
	return nil
}

func (i *builderImage) Order() dist.Order {
	return i.order
}

func (i *builderImage) SetOrder(order dist.Order) error {
	if err := image.MarshalToLabel(i, orderLabel, order); err != nil {
		return err
	}
	i.order = order
	return nil
}

func (i *builderImage) BuildpackLayers() BuildpackLayers {
	return i.bpLayers
}

func (i *builderImage) SetBuildpackLayers(bpLayers BuildpackLayers) error {
	if err := image.MarshalToLabel(i, buildpackLayersLabel, bpLayers); err != nil {
		return err
	}
	i.bpLayers = bpLayers
	return nil
}
