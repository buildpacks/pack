package project

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/project/common"
	v01 "github.com/buildpacks/pack/project/v01"
	v02 "github.com/buildpacks/pack/project/v02"
)

var supportedSchemas = map[string]common.ProjectDescriptorSchema{
	"0.1": v01.Descriptor{},
	"0.2": v02.Descriptor{},
}

type Project struct {
	Version string `toml:"schema-version"`
}
type VersionDescriptor struct {
	Project Project `toml:"_"`
}

func ReadProjectDescriptor(pathToFile string) (common.Descriptor, error) {
	projectTomlContents, err := ioutil.ReadFile(filepath.Clean(pathToFile))
	if err != nil {
		return common.Descriptor{}, err
	}

	var versionDescriptor VersionDescriptor
	_, err = toml.Decode(string(projectTomlContents), &versionDescriptor)
	if err != nil {
		return common.Descriptor{}, errors.Wrapf(err, "parsing schema version")
	}

	var schema common.ProjectDescriptorSchema
	var ok bool
	if versionDescriptor.Project.Version == "" {
		// _.schema-version does not exist in 0.1
		schema = supportedSchemas["0.1"]
	} else {
		if _, err := api.NewVersion(versionDescriptor.Project.Version); err != nil {
			return common.Descriptor{}, err
		}
		schema, ok = supportedSchemas[versionDescriptor.Project.Version]
		if !ok {
			return common.Descriptor{}, fmt.Errorf("unknown project descriptor schema version %s", versionDescriptor.Project.Version)
		}
	}

	descriptor, err := schema.DescriptorFromToml(string(projectTomlContents))
	if err != nil {
		return descriptor, err
	}

	err = validate(descriptor)

	return descriptor, err
}

func validate(p common.Descriptor) error {
	if p.Build.Exclude != nil && p.Build.Include != nil {
		return errors.New("project.toml: cannot have both include and exclude defined")
	}

	if len(p.Project.Licenses) > 0 {
		for _, license := range p.Project.Licenses {
			if license.Type == "" && license.URI == "" {
				return errors.New("project.toml: must have a type or uri defined for each license")
			}
		}
	}

	for _, bp := range p.Build.Buildpacks {
		if bp.ID == "" && bp.URI == "" {
			return errors.New("project.toml: buildpacks must have an id or url defined")
		}
		if bp.URI != "" && bp.Version != "" {
			return errors.New("project.toml: buildpacks cannot have both uri and version defined")
		}
	}

	return nil
}
