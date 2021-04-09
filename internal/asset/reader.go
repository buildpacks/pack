package asset

import (
	"io"
	"sort"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/dist"
)

type Reader struct{}

//go:generate mockgen -package testmocks -destination testmocks/mock_readable.go github.com/buildpacks/pack/internal/asset Readable
type Readable interface {
	Label(string) (string, error)
	GetLayer(diffID string) (io.ReadCloser, error)
}

func NewReader() Reader {
	return Reader{}
}

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
				Blob: Wrapper{
					Func: rd.GetLayer,
					Arg:  asset.LayerDiffID,
				},
				Assets: []dist.Asset{asset.ToAsset(sha256)},
			}
		}
	}

	// TODO -Dan- need to iterate over in a consistent order here
	for _, assetPair := range diffIDMap {
		for _, asset := range assetPair.Assets {
			aBlob, err := ExtractFromLayer(asset, assetPair.Blob)
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
	Assets []dist.Asset
}
