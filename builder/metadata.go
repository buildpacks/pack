package builder

const OrderLabel = "io.buildpacks.buildpack.order"
const BuildpackLayersLabel = "io.buildpacks.buildpack.layers"

type BuildpackLayers map[string]map[string]BuildpackLayerInfo

type BuildpackLayerInfo struct {
	LayerDigest string `json:"layerDigest"`
	Order       Order  `json:"order,omitempty"`
}

type Metadata struct {
	Description string              `json:"description"`
	Buildpacks  []BuildpackMetadata `json:"buildpacks"`
	Groups      V1Order             `json:"groups"` // deprecated
	Stack       StackMetadata       `json:"stack"`
	Lifecycle   LifecycleMetadata   `json:"lifecycle"`
	CreatedBy   CreatorMetadata     `json:"createdBy"`
}

type CreatorMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type BuildpackMetadata struct {
	BuildpackInfo
	Latest bool `json:"latest"` // deprecated
}

type LifecycleMetadata struct {
	LifecycleInfo
	API LifecycleAPI `json:"api"`
}

type StackMetadata struct {
	RunImage RunImageMetadata `json:"runImage" toml:"run-image"`
}

type RunImageMetadata struct {
	Image   string   `json:"image" toml:"image"`
	Mirrors []string `json:"mirrors" toml:"mirrors"`
}

func processMetadata(md *Metadata) error {
	for i, bp := range md.Buildpacks {
		var matchingBps []BuildpackInfo
		for _, bp2 := range md.Buildpacks {
			if bp.ID == bp2.ID {
				matchingBps = append(matchingBps, bp.BuildpackInfo)
			}
		}

		if len(matchingBps) == 1 {
			md.Buildpacks[i].Latest = true
		}
	}

	return nil
}
