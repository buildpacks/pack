// Package asset provides primitives for writing, and reading assets from
// a variety of formats.
package asset

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_blob.go github.com/buildpacks/pack/internal/asset Blob
// Blob is an interface to both the descriptive info and underlying data for
// an asset.
type Blob interface {
	AssetDescriptor() dist.AssetInfo
	Size() int64
	Open() (io.ReadCloser, error)
}

// Singular blob is a concrete implementation of Blob
type SingularBlob struct {
	openFn     func() (io.ReadCloser, error)
	size       int64
	descriptor dist.AssetInfo
}

// Open returns a ReadCloser to contents of an asset.
func (b SingularBlob) Open() (io.ReadCloser, error) {
	return b.openFn()
}

// Size returns the number of bytes in an asset.
func (b SingularBlob) Size() int64 {
	return b.size
}

// AssetDescriptor returns a descriptive struct with metadata about an asset.
func (b SingularBlob) AssetDescriptor() dist.AssetInfo {
	return b.descriptor
}

// FromRawBlob takes metadata about an asset, and a blob that contains asset contents,
// and wraps them together giving us back a SingularBlob.
func FromRawBlob(asset dist.AssetInfo, b blob.Blob) (*SingularBlob, error) {
	result := SingularBlob{
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

// ExtractFromLayers takes metadata about an asset, and layer from an Asset Package (note,
// a single layer may contain multiple assets) and searches the layer for the specified asset.
func ExtractFromLayer(asset dist.AssetInfo, layerBlob blob.Blob) (*SingularBlob, error) {
	pathRegex, err := regexp.Compile(fmt.Sprintf(`(Files)?/cnb/assets/%s`, asset.Sha256))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create asset search regex")
	}

	return FromRawBlob(asset, &SingularBlob{
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
	switch len(readerMap) {
	case 1:
		for _, assetBytes := range readerMap {
			return ioutil.NopCloser(bytes.NewReader(assetBytes)), nil
		}
	default:
		return nil, errors.New(`unable to find singular asset in blob`)
	}

	return nil, nil // impossible to get here.
}
