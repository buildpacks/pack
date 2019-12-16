package libproject

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
)

type Buildpack struct {
	Id      string `toml:"id"`
	Version string `toml:"version"`
	Uri     string `toml:"uri"`
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

type Project struct {
	Name string `toml:"name"`
}

type ProjectDescriptor struct {
	Project  Project                `toml:"project"`
	Build    Build                  `toml:"build"`
	Metadata map[string]interface{} `toml:"metadata"`
}

func ReadProjectDescriptor(pathToFile string) (ProjectDescriptor, error) {
	if _, err := os.Stat(pathToFile); os.IsNotExist(err) {
		return ProjectDescriptor{}, err
	} else {
		projectTomlContents, err := ioutil.ReadFile(pathToFile)
		if err != nil {
			fmt.Print(err)
		}

		var descriptor ProjectDescriptor
		_, err = toml.Decode(string(projectTomlContents), &descriptor)
		if err != nil {
			return ProjectDescriptor{}, err
		}

		return descriptor, descriptor.validate()
	}
}

func (p ProjectDescriptor) validate() error {
	// TODO
	return nil
}