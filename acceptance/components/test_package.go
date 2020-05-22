// +build acceptance

package components

import (
	"fmt"
	"path/filepath"

	h "github.com/buildpacks/pack/testhelpers"
	"github.com/docker/docker/client"
)

type PackageTarget interface {
	Name(workingDir string) string
	Args() []string
	Cleanup()
}

type packageImageConfig struct {
	registry  *h.TestRegistryConfig
	dockerCli *client.Client
	baseName  string
}

func NewPackageImageConfig(
	registry *h.TestRegistryConfig,
	dockerCli *client.Client,
) packageImageConfig {
	return packageImageConfig{
		registry:  registry,
		dockerCli: dockerCli,
		baseName:  randomBuildpackName(),
	}
}

func (p packageImageConfig) Name(workingDir string) string {
	return p.registry.RepoName(p.baseName)
}

func (p packageImageConfig) Args() []string {
	return []string{}
}

func (p packageImageConfig) Cleanup() {
	h.DockerRmi(p.dockerCli, p.registry.RepoName(p.baseName))
}

type packageFileConfig struct {
	baseName string
}

func NewPackageFileConfig() packageFileConfig {
	return packageFileConfig{
		baseName: randomBuildpackName(),
	}
}

func (p packageFileConfig) Name(workingDir string) string {
	return filepath.Join(workingDir, fmt.Sprintf("%s.cnb", p.baseName))
}

func (p packageFileConfig) Args() []string {
	return []string{"--format", "file"}
}

func (p packageFileConfig) Cleanup() {}

func randomBuildpackName() string {
	return fmt.Sprintf("buildpack-%s", h.RandString(8))
}
