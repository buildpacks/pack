// +build acceptance

package managers

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/components"

	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/builder"
)

type BuildpackManager struct {
	testObject *testing.T
	assert     assertions.AssertionManager
	source     string
}

func NewBuildpackManager(t *testing.T, assert assertions.AssertionManager) BuildpackManager {
	buildpackAPI := api.MustParse(builder.DefaultBuildpackAPIVersion).String()

	return BuildpackManager{
		testObject: t,
		assert:     assert,
		source:     filepath.Join("testdata", "mock_buildpacks", buildpackAPI),
	}
}

func (b BuildpackManager) PlaceBuildpacksInDir(destination string, buildpacks ...components.TestBuildpack) {
	b.testObject.Helper()

	for _, buildpack := range buildpacks {
		buildpack.Place(b.testObject, b.assert, b.source, destination)
	}
}

func (b BuildpackManager) FolderBuildpackPath(name string) string {
	return filepath.Join(b.source, name)
}
