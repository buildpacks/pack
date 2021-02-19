package dist

type Assets []Asset

type Asset struct {
	Sha256      string                 `toml:"sha256" json:"sha256,omitempty"`
	ID          string                 `toml:"id" json:"id"`
	Version     string                 `toml:"version" json:"version"`
	Name        string                 `toml:"name" json:"name,omitempty"`
	URI         string                 `toml:"uri" json:"uri,omitempty"`
	Licenses    []string               `toml:"licenses" json:"licenses,omitempty"`
	Description string                 `toml:"description" json:"description,omitempty"`
	Homepage    string                 `toml:"homepage" json:"homepage,omitempty"`
	Stacks      []string               `toml:"stacks" json:"stacks"`
	Metadata    map[string]interface{} `toml:"metadata" json:"metadata,omitempty"`
}

func (a *Asset) ToAssetValue(layerDiffID string) AssetValue {
	return AssetValue{
		ID:          a.ID,
		Version:     a.Version,
		Name:        a.Name,
		LayerDiffID: layerDiffID,
		URI:         a.URI,
		Licenses:    a.Licenses,
		Description: a.Description,
		Homepage:    a.Homepage,
		Stacks:      a.Stacks,
		Metadata:    a.Metadata,
	}
}

func (a *Assets) ToIncompleteAssetMap() AssetMap {
	result := AssetMap{}
	for _, asset := range *a {
		result[asset.Sha256] = asset.ToAssetValue("")
	}

	return result
}




