// +build acceptance

package components

import (
	"fmt"
	"path/filepath"

	"github.com/docker/docker/client"

	h "github.com/buildpacks/pack/testhelpers"
)

type PackageTarget interface {
	Name(workingDir string) string
	Args() []string
	Cleanup()
}

type PackageImageConfig struct {
	registry  *h.TestRegistryConfig
	dockerCli *client.Client
	baseName  string
}

func NewPackageImageConfig(
	registry *h.TestRegistryConfig,
	dockerCli *client.Client,
) PackageImageConfig {
	return PackageImageConfig{
		registry:  registry,
		dockerCli: dockerCli,
		baseName:  randomBuildpackName(),
	}
}

func (p PackageImageConfig) Name(workingDir string) string {
	return p.registry.RepoName(p.baseName)
}

func (p PackageImageConfig) Args() []string {
	return []string{}
}

func (p PackageImageConfig) Cleanup() {
	h.DockerRmi(p.dockerCli, p.registry.RepoName(p.baseName))
}

type PackageFileConfig struct {
	baseName string
}

func NewPackageFileConfig() PackageFileConfig {
	return PackageFileConfig{
		baseName: randomBuildpackName(),
	}
}

func (p PackageFileConfig) Name(workingDir string) string {
	return filepath.Join(workingDir, fmt.Sprintf("%s.cnb", p.baseName))
}

func (p PackageFileConfig) Args() []string {
	return []string{"--format", "file"}
}

func (p PackageFileConfig) Cleanup() {}

func randomBuildpackName() string {
	return fmt.Sprintf("buildpack-%s", h.RandString(8))
}
