package dist

import (
	"github.com/buildpacks/pack/internal/blob"
	"sort"
)


type BlobAssetPair struct {
	Blob     blob.Blob
	AssetVal AssetValue
}

func NewBlobAssetPair(b blob.Blob, aVal AssetValue) BlobAssetPair {
	return BlobAssetPair{
		Blob: b,
		AssetVal: aVal,
	}
}

type BlobMap map[string]BlobAssetPair

func (b *BlobMap) Keys() []string {
	result := make([]string, len(*b))
	idx := 0
	for key := range *b {
		result[idx] = key
		idx++
	}

	sort.Strings(result)
	return result
}

func (b *BlobMap) AssetMap() AssetMap {
	result := make(AssetMap, len(*b))
	for key, value := range *b {
		result[key] = value.AssetVal
	}

	return result
}
