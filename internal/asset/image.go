package asset

import (
	"io/ioutil"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/dist"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"
)

const LayersLabel = "io.buildpacks.asset.layers"

// Image contains internals needed to write an asset package as an OCI image
// to either the docker daemon or a OCI registry.
type Image struct {
	writer LayerWriter
	imgutil.Image
}

// NewImage is a constructor, and how instances of Image should be created.
func NewImage(img imgutil.Image, assetLayerWriter LayerWriter) *Image {
	return &Image{
		writer: assetLayerWriter,
		Image:  img,
	}
}

// Save writes an asset package as an OCI image to the following locations:
// - tag on the 'img' used in the NewImage constructor
// - each additionalName
// Note that additionalNames must all be valid OCI image tag names.
func (a *Image) Save(additionalNames ...string) error {
	tmpDir, err := ioutil.TempDir("", "create-asset-base-dir-scratch")
	if err != nil {
		return err
	}
	imgOS, err := a.OS()
	if err != nil {
		return errors.Wrap(err, "unable to get asset package os")
	}

	if imgOS == pubcfg.WindowsOS {
		err = dist.AddWindowsShimBaseLayer(a, tmpDir)
		if err != nil {
			return errors.Wrap(err, "unable to write windows base layer")
		}
	}

	if err := a.writer.Open(); err != nil {
		return errors.Wrap(err, "unable to open asset writer")
	}
	defer a.writer.Close()

	err = a.writer.Write(a)
	if err != nil {
		return errors.Wrap(err, "unable to write asset layers to image")
	}

	return a.Image.Save(additionalNames...)
}

// AddAssetBlobs adds a list of assetBlobs to the image.
// Note each of these blobs must be 'openable' when Save is called.
func (a *Image) AddAssetBlobs(assetBlob ...Blob) {
	a.writer.AddAssetBlobs(assetBlob...)
}
