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

type BuildImage interface {
	imgutil.Image
	StackID() string
	CommonMixins() []string
	BuildOnlyMixins() []string
}

// TODO: Test this
type ConcreteImage struct {
	BuildImage
	metadata Metadata
	order    dist.Order
	bpLayers BuildpackLayers
	stackID  string
}

func NewBuilderImage(img BuildImage) (*ConcreteImage, error) {
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

	return &ConcreteImage{
		BuildImage: img,
		metadata:   metadata,
		order:      order,
		bpLayers:   bpLayers,
		stackID:    img.StackID(),
	}, nil
}

func (i *ConcreteImage) Metadata() Metadata {
	return i.metadata
}

func (i *ConcreteImage) SetMetadata(metadata Metadata) error {
	if err := image.MarshalToLabel(i, metadataLabel, metadata); err != nil {
		return err
	}
	i.metadata = metadata
	return nil
}

func (i *ConcreteImage) Order() dist.Order {
	return i.order
}

func (i *ConcreteImage) SetOrder(order dist.Order) error {
	if err := image.MarshalToLabel(i, orderLabel, order); err != nil {
		return err
	}
	i.order = order
	return nil
}

func (i *ConcreteImage) BuildpackLayers() BuildpackLayers {
	return i.bpLayers
}

func (i *ConcreteImage) SetBuildpackLayers(bpLayers BuildpackLayers) error {
	if err := image.MarshalToLabel(i, buildpackLayersLabel, bpLayers); err != nil {
		return err
	}
	i.bpLayers = bpLayers
	return nil
}
