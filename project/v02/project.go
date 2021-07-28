package v02

import (
	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/project/common"
)

type Buildpacks struct {
	Include []string           `toml:"include"`
	Exclude []string           `toml:"exclude"`
	Group   []common.Buildpack `toml:"group"`
	Env     Env                `toml:"env"`
	Builder string             `toml:"builder"`
}

type Env struct {
	Build []common.EnvVar `toml:"build"`
}
type Project struct {
	Name          string                 `toml:"name"`
	Licenses      []dist.License         `toml:"licenses"`
	Metadata      map[string]interface{} `toml:"metadata"`
	SchemaVersion string                 `toml:"schema-version"`
}
type IO struct {
	Buildpacks Buildpacks `toml:"buildpacks"`
}
type Descriptor struct {
	Project Project `toml:"_"`
	IO      IO      `toml:"io"`
}

func NewDescriptor() Descriptor {
	return Descriptor{}
}
func (descriptor Descriptor) DescriptorFromToml(projectTomlContents string) (common.Descriptor, error) {
	_, err := toml.Decode(projectTomlContents, &descriptor)
	if err != nil {
		return common.Descriptor{}, err
	}

	var commonDescriptor = common.Descriptor{
		Project: common.Project{
			Name:     descriptor.Project.Name,
			Licenses: descriptor.Project.Licenses,
		},
		Build: common.Build{
			Include:    descriptor.IO.Buildpacks.Include,
			Exclude:    descriptor.IO.Buildpacks.Exclude,
			Buildpacks: descriptor.IO.Buildpacks.Group,
			Env:        descriptor.IO.Buildpacks.Env.Build,
			Builder:    descriptor.IO.Buildpacks.Builder,
		},
		Metadata:      descriptor.Project.Metadata,
		SchemaVersion: api.MustParse("0.2"),
	}
	return commonDescriptor, nil
}
