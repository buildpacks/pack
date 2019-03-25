package builder

import (
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack/buildpack"
)

const MetadataLabel = "io.buildpacks.builder.metadata"

type TOML struct {
	Buildpacks []buildpack.Buildpack      `toml:"buildpacks"`
	Groups     []lifecycle.BuildpackGroup `toml:"groups"`
	Stack      Stack
}

type Stack struct {
	ID              string   `toml:"id"`
	BuildImage      string   `toml:"build-image"`
	RunImage        string   `toml:"run-image"`
	RunImageMirrors []string `toml:"run-image-mirrors,omitempty"`
}

type Metadata struct {
	RunImage   RunImageMetadata    `json:"runImage"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	Groups     []GroupMetadata     `json:"groups"`
}

type RunImageMetadata struct {
	Image   string   `json:"image"`
	Mirrors []string `json:"mirrors"`
}

type BuildpackMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

type GroupMetadata struct {
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
}
