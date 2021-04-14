package asset

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"regexp"

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

func FromRawBlob(asset dist.Asset, b blob.Blob) (*ABlob, error) {
	result := ABlob{
		openFn:     b.Open,
		size:       0,
		descriptor: asset,
	}
	r, err := result.Open()
	if err != nil {
		return nil, err
	}
	w, err := io.Copy(ioutil.Discard, r)
	if err != nil {
		return nil, err
	}
	result.size = w

	return &result, nil
}

func ExtractFromLayer(asset dist.Asset, layerBlob blob.Blob) (*ABlob, error) {
	pathRegex, err := regexp.Compile(fmt.Sprintf(`(Files)?/cnb/assets/%s`, asset.Sha256))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create asset search regex")
	}

	return FromRawBlob(asset, &ABlob{
		openFn: func() (io.ReadCloser, error) {
			return getSingleTarEntry(layerBlob, pathRegex)
		},
	})
}

func getSingleTarEntry(layerBlob blob.Blob, pathRegex *regexp.Regexp) (io.ReadCloser, error) {
	r, err := layerBlob.Open()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to open blob for extraction")
	}

	readerMap, err := archive.ReadMatchingTarEntries(r, pathRegex)
	if err != nil {
		return nil, errors.Wrap(err, "unable to extract single tar entry")
	}
	switch len(readerMap){
	case 1:
		for _, assetBytes := range readerMap {
			return ioutil.NopCloser(bytes.NewReader(assetBytes)), nil
		}
	default:
		return nil, errors.New(`unable to find singular asset in blob`)
	}

	return nil,nil // impossible to get here.
}
