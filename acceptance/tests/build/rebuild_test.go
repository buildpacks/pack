//go:build acceptance
// +build acceptance

package build_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_app_image_is_runnable_and_rebuildable(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	registry := th.Registry()
	imageManager := th.ImageManager()
	runImageName := th.RunImageName()
	runImageMirror := th.RunImageMirror()

	assert := h.NewAssertionManager(t)
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack := combo.Pack()

	appPath := filepath.Join("..", "..", "testdata", "mock_app")

	repo := "some-org/" + h.RandString(10)
	repoName := registry.RepoName(repo)

	output := pack.RunSuccessfully(
		"build", repoName,
		"-p", appPath,
	)

	assertOutput := assertions.NewOutputAssertionManager(t, output)

	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertOutput.ReportsUsingBuildCacheVolume()
	assertOutput.ReportsSelectingRunImageMirror(runImageMirror)

	t.Log("app is runnable")
	assertImage.RunsWithOutput(repoName, "Launch Dep Contents", "Cached Dep Contents")

	t.Log("it uses the run image as a base image")
	assertImage.HasBaseImage(repoName, runImageName)

	t.Log("sets the run image metadata")
	assertImage.HasLabelWithData(repoName, "io.buildpacks.lifecycle.metadata", fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImageName, runImageMirror))

	t.Log("sets the source metadata")
	assertImage.HasLabelWithData(repoName, "io.buildpacks.project.metadata", (`{"source":{"type":"project","version":{"declared":"1.0.2"},"metadata":{"url":"https://github.com/buildpacks/pack"}}}`))

	t.Log("registry is empty")
	assertImage.NotExistsInRegistry(repo)

	t.Log("add a local mirror")
	localRunImageMirror := registry.RepoName("pack-test/run-mirror")
	imageManager.TagImage(runImageName, localRunImageMirror)
	defer imageManager.CleanupImages(localRunImageMirror)

	pack.JustRunSuccessfully("config", "run-image-mirrors", "add", runImageName, "-m", localRunImageMirror)
	defer pack.JustRunSuccessfully("config", "run-image-mirrors", "remove", runImageName)

	t.Log("rebuild")
	output = pack.RunSuccessfully(
		"build", repoName,
		"-p", appPath,
	)
	assertOutput = assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirror)
	cachedLaunchLayer := "simple/layers:cached-launch-layer"

	assertLifecycleOutput := assertions.NewLifecycleOutputAssertionManager(t, output)
	assertLifecycleOutput.ReportsRestoresCachedLayer(cachedLaunchLayer)
	assertLifecycleOutput.ReportsExporterReusingUnchangedLayer(cachedLaunchLayer)
	assertLifecycleOutput.ReportsCacheReuse(cachedLaunchLayer)

	t.Log("app is runnable")
	assertImage.RunsWithOutput(repoName, "Launch Dep Contents", "Cached Dep Contents")

	t.Log("rebuild with --clear-cache")
	output = pack.RunSuccessfully("build",
		repoName,
		"-p", appPath,
		"--clear-cache",
	)

	assertOutput = assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertLifecycleOutput = assertions.NewLifecycleOutputAssertionManager(t, output)
	assertLifecycleOutput.ReportsExporterReusingUnchangedLayer(cachedLaunchLayer)
	assertLifecycleOutput.ReportsCacheCreation(cachedLaunchLayer)

	t.Log("cacher adds layers")
	assert.Matches(output, regexp.MustCompile(`(?i)Adding cache layer 'simple/layers:cached-launch-layer'`))
}
