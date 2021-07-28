package common

import (
	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/dist"
)

type Script struct {
	API    string `toml:"api"`
	Inline string `toml:"inline"`
	Shell  string `toml:"shell"`
}
type Buildpack struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	URI     string `toml:"uri"`
	Script  Script `toml:"script"`
}

type EnvVar struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

type Build struct {
	Include    []string    `toml:"include"`
	Exclude    []string    `toml:"exclude"`
	Buildpacks []Buildpack `toml:"buildpacks"`
	Env        []EnvVar    `toml:"env"`
	Builder    string      `toml:"builder"`
}

type Project struct {
	Name     string         `toml:"name"`
	Licenses []dist.License `toml:"licenses"`
}

type Descriptor struct {
	Project       Project                `toml:"project"`
	Build         Build                  `toml:"build"`
	Metadata      map[string]interface{} `toml:"metadata"`
	SchemaVersion *api.Version
}

type ProjectDescriptorSchema interface {
	DescriptorFromToml(projectTomlContents string) (Descriptor, error)
}
