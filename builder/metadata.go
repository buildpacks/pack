package builder

import (
	"github.com/buildpack/lifecycle"

	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/stack"
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
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
}
