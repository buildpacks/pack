package dist

import (
	"sort"
)

type AssetMap map[string]AssetValue

type AssetValue struct {
	ID          string                 `toml:"id" json:"id"`
	Version     string                 `toml:"version" json:"version"`
	Name        string                 `toml:"name" json:"name,omitempty"`
	LayerDiffID string                 `json:"layerDiffId,omitempty"`
	URI         string                 `toml:"uri" json:"uri,omitempty"`
	Licenses    []string               `toml:"licenses" json:"licenses,omitempty"`
	Description string                 `toml:"description" json:"description,omitempty"`
	Homepage    string                 `toml:"homepage" json:"homepage,omitempty"`
	Stacks      []string               `toml:"stacks" json:"stacks"`
	Metadata    map[string]interface{} `toml:"metadata" json:"metadata,omitempty"`
}

func (a *AssetValue) ToAsset(sha256 string) Asset {
	return Asset{
		Sha256:      sha256,
		ID:          a.ID,
		Version:     a.Version,
		Name:        a.Name,
		URI:         a.URI,
		Licenses:    a.Licenses,
		Description: a.Description,
		Homepage:    a.Homepage,
		Stacks:      a.Stacks,
		Metadata:    a.Metadata,
	}
}

func (a *AssetMap) ToAssets() Assets {
	result := make(Assets, 0)
	for hash, assetVal := range *a {
		result = append(result, assetVal.ToAsset(hash))
	}

	// sort by sha256 to guarantee stability.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Sha256 < result[j].Sha256
	})

	return result
}

func (a *AssetMap) Keys() []string {
	result := make([]string, len(*a))
	idx := 0
	for key := range *a {
		result[idx] = key
		idx++
	}

	sort.Strings(result)
	return result
}

func (a *AssetMap) Filter(keepKeys []string) {
	allKeys := a.Keys()
	sort.Strings(keepKeys)
	// both strings are sorted we can compare using indices
	i := 0
	j := 0
	for j < len(allKeys) {
		switch {
		case i == len(keepKeys) || allKeys[j] < keepKeys[i]:
			delete(*a, allKeys[j])
			j++
		case keepKeys[i] == allKeys[j]:
			i++
			j++
		default: // keepKeys[i] < allKeys[j]:
			i++
		}
	}
}
