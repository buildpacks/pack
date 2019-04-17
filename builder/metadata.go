package builder

import (
	"github.com/buildpack/pack/stack"
)

const MetadataLabel = "io.buildpacks.builder.metadata"

type Metadata struct {
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	Groups     []GroupMetadata     `json:"groups"`
	Stack      stack.Metadata      `json:"stack"`
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
