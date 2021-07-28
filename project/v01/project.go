package v01

import (
	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/project/common"
)

type Descriptor struct {
	Project  common.Project         `toml:"project"`
	Build    common.Build           `toml:"build"`
	Metadata map[string]interface{} `toml:"metadata"`
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
		Project:       descriptor.Project,
		Build:         descriptor.Build,
		Metadata:      descriptor.Metadata,
		SchemaVersion: api.MustParse("0.1"),
	}
	return commonDescriptor, nil
}
