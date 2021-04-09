package asset

import (
	"bytes"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_blob.go github.com/buildpacks/pack/internal/asset Blob
type Blob interface {
	AssetDescriptor() dist.Asset
	Size() int64
	Open() (io.ReadCloser, error)
}

type ABlob struct {
	openFn     func() (io.ReadCloser, error)
	size       int64
	descriptor dist.Asset
}

func (b ABlob) Open() (io.ReadCloser, error) {
	return b.openFn()
}

func (b ABlob) Size() int64 {
	return b.size
}

func (b ABlob) AssetDescriptor() dist.Asset {
	return b.descriptor
}

func FromRawBlob(asset dist.Asset, b blob.Blob) *ABlob {
	result := ABlob{
		openFn:     b.Open,
		size:       0,
		descriptor: asset,
	}
	r, err := result.Open()
	if err != nil {
		// TODO -Dan- handle error
		panic(err)
	}
	w, err := io.Copy(ioutil.Discard, r)
	if err != nil {
		// TODO -Dan- handle error
		panic(err)
	}
	result.size = w

	return &result
}

func ExtractFromLayer(asset dist.Asset, layerBlob blob.Blob) (*ABlob, error) {
	r, err := layerBlob.Open()
	if err != nil {
		return nil, errors.Wrap(err, "unable to open blob for extraction")
	}

	// TODO -Dan- watch out for int32 max size assetBytes here (around 2GB),
	//   rework this using readers to avoid this problem.
	_, assetBytes, err := archive.ReadTarEntry(r, filepath.Join("/cnb", "assets", asset.Sha256))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find asset with sha256: %q in blob", asset.Sha256)
	}
	return FromRawBlob(asset, &ABlob{
		openFn: func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewBuffer(assetBytes)), nil
		},
	}), nil
}
