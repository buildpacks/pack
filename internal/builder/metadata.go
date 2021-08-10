package builder

import "github.com/buildpacks/pack/internal/dist"

const (
	OrderLabel = "io.buildpacks.buildpack.order"
)

type Metadata struct {
	Description string               `json:"description"`
	Buildpacks  []dist.BuildpackInfo `json:"buildpacks"`
	Stack       StackMetadata        `json:"stack"`
	Lifecycle   LifecycleMetadata    `json:"lifecycle"`
	CreatedBy   CreatorMetadata      `json:"createdBy"`
}

type CreatorMetadata struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
}

type LifecycleMetadata struct {
	LifecycleInfo
	// Deprecated: use APIs instead
	API  LifecycleAPI  `json:"api"`
	APIs LifecycleAPIs `json:"apis"`
}

type StackMetadata struct {
	RunImage RunImageMetadata `json:"runImage" toml:"run-image"`
}

type ProjectMetadata struct {
	Source ProjectSource `json:"source,omitempty" toml:"source"`
}

type ProjectSource struct {
	Type     string                 `toml:"type" json:"type,omitempty"`
	Version  map[string]interface{} `toml:"version" json:"version,omitempty"`
	Metadata map[string]interface{} `toml:"metadata" json:"metadata,omitempty"`
}
type RunImageMetadata struct {
	Image   string   `json:"image" toml:"image"`
	Mirrors []string `json:"mirrors" toml:"mirrors"`
}
