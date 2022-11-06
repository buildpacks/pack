//go:build acceptance
// +build acceptance

package build_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_arg_no_color(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	pack := combo.Pack()

	appPath := filepath.Join("..", "..", "testdata", "mock_app")

	registry := th.Registry()
	repoName := registry.RepoName("sample/" + h.RandString(3))

	output := pack.RunSuccessfully("build", repoName, "-p", appPath, "--no-color")
	assertOutput := assertions.NewOutputAssertionManager(t, output)
	assertOutput.ReportsSuccessfulImageBuild(repoName)
	assertOutput.WithoutColors()
}
