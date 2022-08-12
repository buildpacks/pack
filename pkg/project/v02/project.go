package v02

import (
	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/pkg/project/types"
)

type Buildpacks struct {
	Include []string          `toml:"include"`
	Exclude []string          `toml:"exclude"`
	Group   []types.Buildpack `toml:"group"`
	Env     Env               `toml:"env"`
	Build   Build             `toml:"build"`
	Builder string            `toml:"builder"`
}

type Build struct {
	Env []types.EnvVar `toml:"env"`
}

// deprecated: use `[[io.buildpacks.build.env]]` instead. see https://github.com/buildpacks/pack/pull/1479
type Env struct {
	Build []types.EnvVar `toml:"build"`
}

type Project struct {
	Name          string                 `toml:"name"`
	Licenses      []types.License        `toml:"licenses"`
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

func NewDescriptor(projectTomlContents string) (types.Descriptor, error) {
	versionedDescriptor := &Descriptor{}
	_, err := toml.Decode(projectTomlContents, &versionedDescriptor)
	if err != nil {
		return types.Descriptor{}, err
	}

	// backward compatibility for incorrect key
	env := versionedDescriptor.IO.Buildpacks.Build.Env
	if env == nil {
		env = versionedDescriptor.IO.Buildpacks.Env.Build
	}

	return types.Descriptor{
		Project: types.Project{
			Name:     versionedDescriptor.Project.Name,
			Licenses: versionedDescriptor.Project.Licenses,
		},
		Build: types.Build{
			Include:    versionedDescriptor.IO.Buildpacks.Include,
			Exclude:    versionedDescriptor.IO.Buildpacks.Exclude,
			Buildpacks: versionedDescriptor.IO.Buildpacks.Group,
			Env:        env,
			Builder:    versionedDescriptor.IO.Buildpacks.Builder,
		},
		Metadata:      versionedDescriptor.Project.Metadata,
		SchemaVersion: api.MustParse("0.2"),
	}, nil
}
