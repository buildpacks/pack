package dist

import "github.com/buildpack/pack/internal/api"

const BuildpackLayersLabel = "io.buildpacks.buildpack.layers"

type BuildpackURI struct {
	URI string `toml:"uri"`
}

type ImageRef struct {
	Ref string `toml:"ref"`
}

type Order []OrderEntry

type OrderEntry struct {
	Group []BuildpackRef `toml:"group" json:"group"`
}

type BuildpackRef struct {
	BuildpackInfo
	Optional bool `toml:"optional,omitempty" json:"optional,omitempty"`
}

type BuildpackLayers map[string]map[string]BuildpackLayerInfo

type BuildpackLayerInfo struct {
	API         *api.Version `json:"api"`
	Stacks      []Stack      `json:"stacks"`
	Order       Order        `json:"order,omitempty"`
	LayerDiffID string       `json:"layerDiffID"`
}
