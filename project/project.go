package project

import (
	"io/ioutil"

	ignore "github.com/sabhiram/go-gitignore"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Buildpack struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	URI     string `toml:"uri"`
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
}

type License struct {
	Type string `toml:"type"`
	URI  string `toml:"uri"`
}

type Project struct {
	Name     string    `toml:"name"`
	Licenses []License `toml:"licenses"`
}

type Descriptor struct {
	Project  Project                `toml:"project"`
	Build    Build                  `toml:"build"`
	Metadata map[string]interface{} `toml:"metadata"`
}

func ReadProjectDescriptor(pathToFile string) (Descriptor, error) {
	projectTomlContents, err := ioutil.ReadFile(pathToFile)
	if err != nil {
		return Descriptor{}, err
	}

	var descriptor Descriptor
	_, err = toml.Decode(string(projectTomlContents), &descriptor)
	if err != nil {
		return Descriptor{}, err
	}

	return descriptor, descriptor.validate()
}

func (d *Descriptor) GetFileFilter() (func(string) bool, error) {
	if len(d.Build.Exclude) > 0 {
		excludes, err := ignore.CompileIgnoreLines(d.Build.Exclude...)
		if err != nil {
			return nil, err
		}
		return func(fileName string) bool {
			return !excludes.MatchesPath(fileName)
		}, nil
	}
	if len(d.Build.Include) > 0 {
		includes, err := ignore.CompileIgnoreLines(d.Build.Include...)
		if err != nil {
			return nil, err
		}
		return includes.MatchesPath, nil
	}

	return nil, nil
}

func (d *Descriptor) validate() error {
	if d.Build.Exclude != nil && d.Build.Include != nil {
		return errors.New("project.toml: cannot have both include and exclude defined")
	}
	if len(d.Project.Licenses) > 0 {
		for _, license := range d.Project.Licenses {
			if license.Type == "" && license.URI == "" {
				return errors.New("project.toml: must have a type or uri defined for each license")
			}
		}
	}

	for _, bp := range d.Build.Buildpacks {
		if bp.ID == "" && bp.URI == "" {
			return errors.New("project.toml: buildpacks must have an id or url defined")
		}
		if bp.URI != "" && bp.Version != "" {
			return errors.New("project.toml: buildpacks cannot have both uri and version defined")
		}
	}

	return nil
}
