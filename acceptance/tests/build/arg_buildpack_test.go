//go:build acceptance
// +build acceptance

package build_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/buildpacks"
	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_arg_buildpack(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	pack := combo.Pack()
	lifecycle := combo.Lifecycle()

	assert := h.NewAssertionManager(t)
	registry := th.Registry()
	imageManager := th.ImageManager()
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	appPath := filepath.Join("..", "..", "testdata", "mock_app")
	repoName := registry.RepoName("test/" + h.RandString(10))

	buildpackManager := buildpacks.NewBuildpackManager(
		t,
		assert,
		buildpacks.WithBuildpackAPIVersion(lifecycle.EarliestBuildpackAPIVersion()),
		buildpacks.WithBaseDir(filepath.Join("..", "..", "testdata", "mock_buildpacks")),
	)

	t.Run("the argument is an ID", func(t *testing.T) {
		output := pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--buildpack", "simple/layers", // can omit version if only one
			"--buildpack", "noop.buildpack@noop.buildpack.version",
		)

		assertOutput := assertions.NewOutputAssertionManager(t, output)

		assertTestAppOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
		assertTestAppOutput.ReportsBuildStep("Simple Layers Buildpack")
		assertTestAppOutput.ReportsBuildStep("NOOP Buildpack")
		assertOutput.ReportsSuccessfulImageBuild(repoName)

		t.Log("app is runnable")
		assertImage.RunsWithOutput(
			repoName,
			"Launch Dep Contents",
			"Cached Dep Contents",
		)
	})

	t.Run("the argument is an archive", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "archive-buildpack-tests-")
		assert.Nil(err)
		t.Cleanup(func() {
			assert.Succeeds(os.RemoveAll(tmpDir))
		})

		buildpackManager.PrepareBuildpacks(tmpDir, buildpacks.ArchiveNotInBuilder)

		output := pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--buildpack", buildpacks.ArchiveNotInBuilder.FullPathIn(tmpDir),
		)

		assertOutput := assertions.NewOutputAssertionManager(t, output)
		assertOutput.ReportsAddingBuildpack("local/bp", "local-bp-version")
		assertOutput.ReportsSuccessfulImageBuild(repoName)

		assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
		assertBuildpackOutput.ReportsBuildStep("Local Buildpack")
	})

	t.Run("the argument is a directory", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "folder-buildpack-tests-")
		assert.Nil(err)
		t.Cleanup(func() {
			assert.Succeeds(os.RemoveAll(tmpDir))
		})

		h.SkipIf(t, runtime.GOOS == "windows", "buildpack directories not supported on windows")

		buildpackManager.PrepareBuildpacks(tmpDir, buildpacks.FolderNotInBuilder)

		output := pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--buildpack", buildpacks.FolderNotInBuilder.FullPathIn(tmpDir),
		)

		assertOutput := assertions.NewOutputAssertionManager(t, output)
		assertOutput.ReportsAddingBuildpack("local/bp", "local-bp-version")
		assertOutput.ReportsSuccessfulImageBuild(repoName)

		assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
		assertBuildpackOutput.ReportsBuildStep("Local Buildpack")
	})

	t.Run("the argument is a buildpackage image", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "test-buildpackages-")
		assert.Nil(err)

		packageImageName := registry.RepoName("buildpack-" + h.RandString(8))

		t.Cleanup(func() {
			imageManager.CleanupImages(packageImageName)
			assert.Succeeds(os.RemoveAll(tmpDir))
		})

		packageTomlPath := pack.FixtureManager().TemplateFixtureToFile(
			tmpDir,
			"package_for_build_cmd.toml",
			map[string]interface{}{
				"OS": imageManager.HostOS(),
			},
		)

		packageImage := buildpacks.NewPackageImage(
			t,
			pack,
			packageImageName,
			packageTomlPath,
			buildpacks.WithRequiredBuildpacks(
				buildpacks.FolderSimpleLayersParent,
				buildpacks.FolderSimpleLayers,
			),
		)

		buildpackManager.PrepareBuildpacks(tmpDir, packageImage)

		output := pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--buildpack", packageImageName,
		)

		assertOutput := assertions.NewOutputAssertionManager(t, output)
		assertOutput.ReportsAddingBuildpack(
			"simple/layers/parent",
			"simple-layers-parent-version",
		)
		assertOutput.ReportsAddingBuildpack("simple/layers", "simple-layers-version")
		assertOutput.ReportsSuccessfulImageBuild(repoName)

		assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
		assertBuildpackOutput.ReportsBuildStep("Simple Layers Buildpack")
	})

	t.Run("the argument is a buildpackage file", func(t *testing.T) {
		tmpDir, err := ioutil.TempDir("", "package-file")
		assert.Nil(err)

		t.Cleanup(func() {
			assert.Succeeds(os.RemoveAll(tmpDir))
		})

		packageFileLocation := filepath.Join(
			tmpDir,
			fmt.Sprintf("buildpack-%s.cnb", h.RandString(8)),
		)

		packageTomlPath := pack.FixtureManager().TemplateFixtureToFile(
			tmpDir,
			"package_for_build_cmd.toml",
			map[string]interface{}{
				"OS": imageManager.HostOS(),
			},
		)

		packageFile := buildpacks.NewPackageFile(
			t,
			pack,
			packageFileLocation,
			packageTomlPath,
			buildpacks.WithRequiredBuildpacks(
				buildpacks.FolderSimpleLayersParent,
				buildpacks.FolderSimpleLayers,
			),
		)

		buildpackManager.PrepareBuildpacks(tmpDir, packageFile)

		output := pack.RunSuccessfully(
			"build", repoName,
			"-p", appPath,
			"--buildpack", packageFileLocation,
		)

		assertOutput := assertions.NewOutputAssertionManager(t, output)
		assertOutput.ReportsAddingBuildpack(
			"simple/layers/parent",
			"simple-layers-parent-version",
		)
		assertOutput.ReportsAddingBuildpack("simple/layers", "simple-layers-version")
		assertOutput.ReportsSuccessfulImageBuild(repoName)

		assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
		assertBuildpackOutput.ReportsBuildStep("Simple Layers Buildpack")
	})

	t.Run("the buildpack stack doesn't match the builder", func(t *testing.T) {
		bpDir := filepath.Join("..", "..", "testdata", "mock_buildpacks", lifecycle.EarliestBuildpackAPIVersion())
		otherStackBuilderTgz := h.CreateTGZ(t, filepath.Join(bpDir, "other-stack-buildpack"), "./", 0755)

		t.Cleanup(func() {
			assert.Succeeds(os.Remove(otherStackBuilderTgz))
		})

		output, err := pack.Run(
			"build", repoName,
			"-p", appPath,
			"--buildpack", otherStackBuilderTgz,
		)

		assert.NotNil(err)
		assert.Contains(output, "other/stack/bp")
		assert.Contains(output, "other-stack-version")
		assert.Contains(output, "does not support stack 'pack.test.stack'")
	})
}
