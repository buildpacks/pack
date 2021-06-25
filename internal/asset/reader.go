package asset

import (
	"io"
	"sort"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
)

// Reader provides internals needed to read individual assets out
// of an asset package.
type Reader struct{}

//go:generate mockgen -package testmocks -destination testmocks/mock_readable.go github.com/buildpacks/pack/internal/asset Readable

// Readable represents the minimum interface an image like object must have
// in order for assets to be read out of it.
type Readable interface {
	Label(string) (string, error)
	GetLayer(diffID string) (io.ReadCloser, error)
}

// NewReader is the constructor method
// this should be used to create new instances Reader structs.
func NewReader() Reader {
	return Reader{}
}

// Read takes a reable object and reads all assets out of it using
// the "io.buildpacks.asset.layers" image label.
// it returns an ordered list of assets and the metadata.
func (r Reader) Read(rd Readable) ([]Blob, dist.AssetMap, error) {
	md := dist.AssetMap{}
	var blobs []Blob
	if found, err := dist.GetLabel(rd, LayersLabel, &md); err != nil {
		return blobs, md, errors.Wrap(err, "unable to get asset layers label")
	} else if !found {
		return blobs, md, nil
	}

	diffIDMap := map[string]multiAssetLayer{}
	for sha256, asset := range md {
		a, ok := diffIDMap[asset.LayerDiffID]
		if ok {
			a.Assets = append(a.Assets, asset.ToAsset(sha256))
			diffIDMap[asset.LayerDiffID] = a
		} else {
			diffIDMap[asset.LayerDiffID] = multiAssetLayer{
				Blob: wrapper{
					Func: rd.GetLayer,
					Arg:  asset.LayerDiffID,
				},
				Assets: []dist.AssetInfo{asset.ToAsset(sha256)},
			}
		}
	}

	for _, assetLayer := range diffIDMap {
		for _, asset := range assetLayer.Assets {
			aBlob, err := ExtractFromLayer(asset, assetLayer.Blob)
			if err != nil {
				return blobs, md, err
			}
			blobs = append(blobs, aBlob)
		}
	}

	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].AssetDescriptor().Sha256 < blobs[j].AssetDescriptor().Sha256
	})
	return blobs, md, nil
}

type multiAssetLayer struct {
	blob.Blob
	Assets []dist.AssetInfo
}
