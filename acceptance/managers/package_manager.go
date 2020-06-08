// +build acceptance

package managers

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/components"

	"github.com/buildpacks/pack/acceptance/assertions"

	h "github.com/buildpacks/pack/testhelpers"
)

type pack interface {
	FixtureMustExist(string) string
	SuccessfulRun(string, ...string)
}

type PackageManager struct {
	testObject       *testing.T
	assert           assertions.AssertionManager
	pack             pack
	buildpackManager BuildpackManager
	suiteManager     *SuiteManager
	tmpDir           string
}

func NewPackageManager(t *testing.T, assert assertions.AssertionManager, suiteManager *SuiteManager, pack pack) PackageManager {
	t.Helper()

	tmpDir, err := ioutil.TempDir("", "package-manager")
	assert.Nil(err)

	p := PackageManager{
		testObject:       t,
		assert:           assert,
		suiteManager:     suiteManager,
		pack:             pack,
		buildpackManager: NewBuildpackManager(t, assert),
		tmpDir:           tmpDir,
	}

	return p
}

func (p PackageManager) Cleanup() {
	os.RemoveAll(p.tmpDir)
}

//nolint:whitespace // A leading line of whitespace is left after a method declaration with multi-line arguments
func (p PackageManager) PackageBuildpack(
	target components.PackageTarget,
	configFixtureName string,
	buildpacks []components.TestBuildpack,
) string {

	p.testObject.Helper()
	p.testObject.Log("creating package image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir(p.tmpDir, "package-buildpack")
	p.assert.Nil(err)

	p.buildpackManager.PlaceBuildpacksInDir(tmpDir, buildpacks...)

	packageConfigPath := filepath.Join(tmpDir, configFixtureName)
	h.CopyFile(p.testObject, p.pack.FixtureMustExist(configFixtureName), packageConfigPath)

	outputName := target.Name(tmpDir)

	p.pack.SuccessfulRun(
		"package-buildpack",
		append([]string{outputName, "--no-color", "-p", packageConfigPath}, target.Args()...)...,
	)

	p.suiteManager.RegisterCleanUp(fmt.Sprintf("cleanup-package-%s", outputName), target.Cleanup)

	return outputName
}
