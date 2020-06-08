// +build acceptance

package managers

import (
	"io/ioutil"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/components"
	h "github.com/buildpacks/pack/testhelpers"
)

//nolint:whitespace // A leading line of whitespace is left after a method declaration with multi-line arguments
func NewPackExecutor(
	testObject *testing.T,
	assetManager AssetManager,
	kind ComboValue,
	registryConfig *h.TestRegistryConfig,
	assert assertions.AssertionManager,
) *components.PackExecutor {

	testObject.Helper()

	path, fixtures := assetManager.PackPaths(kind)

	home, err := ioutil.TempDir("", "buildpack.pack.home.")
	if err != nil {
		testObject.Fatalf("couldn't create home folder for %s pack: %s", kind, err)
	}

	return components.NewPackExecutor(testObject, path, fixtures, home, registryConfig, assert)
}

func NewTestLifecycle(assetManager AssetManager, kind ComboValue) *components.TestLifecycle {
	path, descriptor := assetManager.Lifecycle(kind)

	return components.NewTestLifecycle(path, descriptor)
}
