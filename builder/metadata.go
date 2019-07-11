package builder

import (
	"github.com/buildpack/pack/lifecycle"
)

const MetadataLabel = "io.buildpacks.builder.metadata"

type Metadata struct {
	Description string              `json:"description"`
	Buildpacks  []BuildpackMetadata `json:"buildpacks"`
	Groups      []GroupMetadata     `json:"groups"`
	Stack       StackMetadata       `json:"stack"`
	Lifecycle   lifecycle.Metadata  `json:"lifecycle"`
}

type BuildpackMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

type GroupMetadata struct {
	Buildpacks []GroupBuildpack `json:"buildpacks" toml:"buildpacks"`
}

type OrderTOML struct {
	Groups []GroupMetadata `toml:"groups"`
}

type GroupBuildpack struct {
	ID       string `json:"id" toml:"id"`
	Version  string `json:"version" toml:"version"`
	Optional bool   `json:"optional,omitempty" toml:"optional,omitempty"`
}

type StackMetadata struct {
	RunImage RunImageMetadata `toml:"run-image" json:"runImage"`
}

type RunImageMetadata struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors"`
}
