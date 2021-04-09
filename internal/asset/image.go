package asset

import (
	"io/ioutil"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/dist"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
)

const LayersLabel = "io.buildpacks.asset.layers"
const AssetHashAlgorithm = "sha256"

type BlobAssetPair struct {
	Blob  blob.Blob
	Asset dist.Asset
}

type Image struct {
	writer LayerWriter
	imgutil.Image
}

func NewImage(img imgutil.Image, assetLayerWriter LayerWriter) *Image {
	return &Image{
		writer: assetLayerWriter,
		Image:  img,
	}
}

func (a *Image) Save(additionalNames ...string) error {
	tmpDir, err := ioutil.TempDir("", "create-asset-base-dir-scratch")
	if err != nil {
		return err
	}
	imgOS, err := a.OS()
	if err != nil {
		return errors.Wrap(err, "unable to get asset cache image os")
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

func (a *Image) AddAssetBlobs(layerBlobs ...Blob) {
	a.writer.AddAssetBlobs(layerBlobs...)
}
