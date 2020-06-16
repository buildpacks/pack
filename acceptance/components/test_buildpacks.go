// +build acceptance

package components

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	h "github.com/buildpacks/pack/testhelpers"
)

type archiveBuildpack struct {
	name string
}

type TestBuildpack interface {
	Place(testObject *testing.T, assert assertions.AssertionManager, sourceDir, destination string)
	BuilderConfigBlock() string
}

type buildpackPackager interface {
	PackageBuildpack(target PackageTarget, configFixtureName string, buildpacks []TestBuildpack) string
}

func (b *archiveBuildpack) Place(testObject *testing.T, assert assertions.AssertionManager, sourceDir, destination string) {
	testObject.Helper()

	err := os.Rename(
		h.CreateTGZ(testObject, filepath.Join(sourceDir, b.name), "./", 0755),
		filepath.Join(destination, b.FileName()),
	)

	assert.Nil(err)
}

func (b *archiveBuildpack) BuilderConfigBlock() string {
	return fmt.Sprintf(`
[[buildpacks]]
  uri = "%s"
`, b.FileName())
}

func (b *archiveBuildpack) FileName() string {
	return fmt.Sprintf("%s.tgz", b.name)
}

type PackageImageBuildpack struct {
	packageManager    buildpackPackager
	config            PackageImageConfig
	configFixtureName string
	buildpacks        []TestBuildpack
}

//nolint:whitespace // A leading line of whitespace is left after a method declaration with multi-line arguments
func NewPackageImageBuildpack(
	packageManager buildpackPackager,
	config PackageImageConfig,
	configFixtureName string,
	buildpacks ...TestBuildpack,
) PackageImageBuildpack {

	return PackageImageBuildpack{
		packageManager:    packageManager,
		config:            config,
		configFixtureName: configFixtureName,
		buildpacks:        buildpacks,
	}
}

func (b PackageImageBuildpack) Place(testObject *testing.T, assert assertions.AssertionManager, sourceDir, destination string) {
	testObject.Helper()

	b.packageManager.PackageBuildpack(b.config, b.configFixtureName, b.buildpacks)
}

func (b PackageImageBuildpack) BuilderConfigBlock() string {
	return fmt.Sprintf(`
[[buildpacks]]
  image = "%s"
`, b.config.Name(""))
}

var (
	SimpleLayersParentBuildpack = &archiveBuildpack{name: "simple-layers-parent-buildpack"}
	SimpleLayersBuildpack       = &archiveBuildpack{name: "simple-layers-buildpack"}
	NoOpBuildpack               = &archiveBuildpack{name: "noop-buildpack"}
	NoOpBuildpack2              = &archiveBuildpack{name: "noop-buildpack-2"}
	OtherStackBuildpack         = &archiveBuildpack{name: "other-stack-buildpack"}
	ReadEnvBuildpack            = &archiveBuildpack{name: "read-env-buildpack"}
	InternetCapableBuildpack    = &archiveBuildpack{name: "internet-capable-buildpack"}
	VolumeBuildpack             = &archiveBuildpack{name: "volume-buildpack"}
	NotInBuilderBuildpack       = &archiveBuildpack{name: "not-in-builder-buildpack"}
	DescriptorBuildpack         = &archiveBuildpack{name: "descriptor-buildpack"}
)
