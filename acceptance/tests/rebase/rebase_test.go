//go:build acceptance
// +build acceptance

package rebase_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestRebase(t *testing.T) {
	testHarness := harness.ContainingBuilder(t, filepath.Join("..", "..", ".."))
	t.Cleanup(testHarness.CleanUp)

	testHarness.RunA(test_run_image_flag)
	testHarness.RunA(test_local_config_mirror)
	testHarness.RunA(test_stack_config_mirror)
}

func test_run_image_flag(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	assert := h.NewAssertionManager(t)

	pack := combo.Pack()
	builderName := combo.BuilderName()

	registry := th.Registry()
	runImageName := th.Stack().RunImage.Name
	imageManager := th.ImageManager()
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack.JustRunSuccessfully("config", "trusted-builders", "add", builderName)

	repoName := registry.RepoName("some-org/" + h.RandString(10))
	originalRunImage := registry.RepoName("acceptance/run-image:original" + h.RandString(10))

	th.ImageManager().CreateImage(
		originalRunImage,
		runImageDockerfile(
			runImageName,
			hostRootUser(imageManager.HostOS()),
			"contents-original-1",
			"contents-original-2",
		),
	)

	pack.RunSuccessfully(
		"build", repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
		"--builder", builderName,
		"--run-image", originalRunImage,
		"--pull-policy", "never",
	)

	originalID := h.ImageID(t, repoName)

	assertImage.RunsWithOutput(
		repoName,
		"contents-original-1",
		"contents-original-2",
	)

	t.Cleanup(func() {
		imageManager.CleanupImages(originalID, repoName, originalRunImage)
	})

	updatedRunImage := registry.RepoName("acceptance/run-image:updated" + h.RandString(10))
	imageManager.CreateImage(
		updatedRunImage,
		runImageDockerfile(
			runImageName,
			hostRootUser(imageManager.HostOS()),
			"contents-updated-1",
			"contents-updated-2",
		))
	t.Cleanup(func() {
		imageManager.CleanupImages(updatedRunImage)
	})

	output := pack.RunSuccessfully(
		"rebase", repoName,
		"--run-image", updatedRunImage,
		"--pull-policy", "never",
	)

	assert.Contains(output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
	assertImage.RunsWithOutput(
		repoName,
		"contents-updated-1",
		"contents-updated-2",
	)
}

func test_local_config_mirror(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	builderName := combo.BuilderName()
	pack := combo.Pack()

	registry := th.Registry()
	runImageName := th.Stack().RunImage.Name
	imageManager := th.ImageManager()

	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	localRunImageMirror := "run-after/" + h.RandString(10)
	imageManager.CreateImage(
		localRunImageMirror,
		runImageDockerfile(
			runImageName,
			hostRootUser(imageManager.HostOS()),
			"local-mirror-after-1",
			"local-mirror-after-2",
		))

	output := pack.RunSuccessfully("config", "run-image-mirrors", "add", runImageName, "-m", localRunImageMirror)
	t.Log(output)

	t.Cleanup(func() {
		imageManager.CleanupImages(localRunImageMirror)
	})

	repoName := "pack-test/rebase-local-config-mirror:app" + h.RandString(6)

	pack.RunSuccessfully(
		"build", repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
		"--builder", builderName,
		"--pull-policy", "never",
	)

	output = pack.RunSuccessfully("rebase", repoName, "--pull-policy", "never")

	assertOutput := assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirror)
	assertOutput.ReportsSuccessfulRebase(repoName)
	assertImage.RunsWithOutput(
		repoName,
		"local-mirror-after-1",
		"local-mirror-after-2",
	)
}

func test_stack_config_mirror(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	builderName := combo.BuilderName()
	pack := combo.Pack()

	registry := th.Registry()
	runImageName := th.Stack().RunImage.Name
	runImageMirror := th.Stack().RunImage.MirrorName
	imageManager := th.ImageManager()

	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	// FIXME: we should not be overriding suite level test asset
	th.ImageManager().CreateImage(runImageMirror,
		runImageDockerfile(runImageName,
			hostRootUser(imageManager.HostOS()),
			"mirror-after-1",
			"mirror-after-2",
		))

	t.Cleanup(func() {
		imageManager.CleanupImages(runImageMirror)
	})

	repoName := registry.RepoName("pack-test/stack-config-run-image:app" + h.RandString(6))

	pack.RunSuccessfully(
		"build", repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
		"--builder", builderName,
		"--pull-policy", "never",
	)

	output := pack.RunSuccessfully("rebase", repoName, "--pull-policy", "never")

	assertOutput := assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSelectingRunImageMirror(runImageMirror)
	assertOutput.ReportsSuccessfulRebase(repoName)
	assertImage.RunsWithOutput(
		repoName,
		"mirror-after-1",
		"mirror-after-2",
	)
}

func runImageDockerfile(baseRunImage, user, contents1, contents2 string) string {
	return fmt.Sprintf(`FROM %s
USER %s
RUN echo %s > /contents1.txt
RUN echo %s > /contents2.txt
USER pack`, baseRunImage, user, contents1, contents2)
}

func hostRootUser(hostOS string) string {
	if hostOS == "windows" {
		return "ContainerAdministrator"
	}

	return "root"
}
