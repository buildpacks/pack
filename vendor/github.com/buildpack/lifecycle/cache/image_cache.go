package cache

import (
	"encoding/json"
	"io"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/metadata"
)

//go:generate mockgen -package testmock -destination testmock/image_factory.go github.com/buildpack/lifecycle/cache ImageFactory
type ImageFactory interface {
	NewEmptyLocal(string) image.Image
}

type ImageCache struct {
	factory   ImageFactory
	origImage image.Image
	newImage  image.Image
}

func NewImageCache(factory ImageFactory, origImage image.Image) *ImageCache {
	newImage := factory.NewEmptyLocal(origImage.Name())
	return &ImageCache{
		factory:   factory,
		origImage: origImage,
		newImage:  newImage,
	}
}

func (c *ImageCache) Name() string {
	return c.origImage.Name()
}

func (c *ImageCache) SetMetadata(metadata Metadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "serializing metadata")
	}
	return c.newImage.SetLabel(MetadataLabel, string(data))
}

func (c *ImageCache) RetrieveMetadata() (Metadata, error) {
	contents, err := metadata.GetRawMetadata(c.origImage, MetadataLabel)
	if err != nil {
		return Metadata{}, errors.Wrap(err, "retrieving metadata")
	}

	meta := Metadata{}
	if json.Unmarshal([]byte(contents), &meta) != nil {
		return Metadata{}, nil
	}
	return meta, nil
}

func (c *ImageCache) AddLayer(identifier string, sha string, tarPath string) error {
	return c.newImage.AddLayer(tarPath)
}

func (c *ImageCache) ReuseLayer(identifier string, sha string) error {
	return c.newImage.ReuseLayer(sha)
}

func (c *ImageCache) RetrieveLayer(sha string) (io.ReadCloser, error) {
	return c.origImage.GetLayer(sha)
}

func (c *ImageCache) Commit() error {
	_, err := c.newImage.Save()
	if err != nil {
		return errors.Wrapf(err, "saving image '%s'", c.newImage.Name())
	}

	if err := c.origImage.Delete(); err != nil {
		return errors.Wrapf(err, "deleting image '%s'", c.origImage.Name())
	}

	c.origImage = c.newImage
	c.newImage = c.factory.NewEmptyLocal(c.origImage.Name())

	return nil
}
