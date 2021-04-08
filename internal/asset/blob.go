package asset

import (
	"bytes"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"path/filepath"
)


//go:generate mockgen -package testmocks -destination testmocks/mock_blob.go github.com/buildpacks/pack/internal/asset Blob
type Blob interface {
	AssetDescriptor() dist.Asset
	Size() int64
	Open() (io.ReadCloser, error)
}

type assetBlob struct {
	openFn     func() (io.ReadCloser, error)
	size       int64
	descriptor dist.Asset
}

func (b assetBlob) Open() (io.ReadCloser, error) {
	return b.openFn()
}

func (b assetBlob) Size() int64 {
	return b.size
}

func (b assetBlob) AssetDescriptor() dist.Asset {
	return b.descriptor
}

// Ways we want to be able to make these blobs,
// 1) from a random blob for an asset we just downloaded
//func FromRawBlobOLD(asset dist.Asset, b blob.Blob, wf *layer.WriterFactory) *assetBlob {
//	result := &assetBlob{openFn: func() (io.ReadCloser, error) {
//		return archive.GenerateTarWithWriter(
//			func(tw archive.TarWriter) error {
//				return toAssetTar(tw, asset.Sha256, b)
//			},
//			wf,
//		), nil
//	},
//		descriptor: asset,
//	}
//	// get size of resulting asset
//	r, err := result.Open()
//	if err != nil {
//		// TODO -Dan- handle error
//		panic(err)
//	}
//	w, err := io.Copy(ioutil.Discard, r)
//	if err != nil {
//		// TODO -Dan- handle error
//		panic(err)
//	}
//	result.size = w
//
//	return result
//}


func FromRawBlob(asset dist.Asset, b blob.Blob) *assetBlob {
	result := assetBlob{
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


func ExtractFromLayer(asset dist.Asset, layerBlob blob.Blob) (*assetBlob, error) {
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
	return FromRawBlob(asset, &assetBlob{
		openFn: func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewBuffer(assetBytes)), nil
		},
	}), nil
}
