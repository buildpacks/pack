//go:build acceptance
// +build acceptance

package build_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/harness"
	"github.com/buildpacks/pack/testhelpers"
)

func test_untrusted_daemon(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	pack := combo.Pack()
	lifecycle := combo.Lifecycle()

	registry := th.Registry()
	repoName := registry.RepoName("sample/" + testhelpers.RandString(3))

	output := pack.RunSuccessfully(
		"build", repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
	)

	assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)

	assertOutput := assertions.NewLifecycleOutputAssertionManager(t, output)
	assertOutput.IncludesLifecycleImageTag(lifecycle.Image())
	assertOutput.IncludesSeparatePhases()
}

func test_untrusted_registry(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	imageManager := th.ImageManager()
	registry := th.Registry()
	repoName := registry.RepoName("sample/" + testhelpers.RandString(3))

	pack := combo.Pack()
	lifecycle := combo.Lifecycle()

	buildArgs := []string{
		repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
		"--publish",
	}

	if imageManager.HostOS() != "windows" {
		buildArgs = append(buildArgs, "--network", "host")
	}

	output := pack.RunSuccessfully("build", buildArgs...)

	assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)

	assertOutput := assertions.NewLifecycleOutputAssertionManager(t, output)
	assertOutput.IncludesLifecycleImageTag(lifecycle.Image())
	assertOutput.IncludesSeparatePhases()
}
