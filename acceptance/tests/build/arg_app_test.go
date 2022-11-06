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

func test_arg_app_zipfile(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	registry := th.Registry()

	pack := combo.Pack()

	repoName := registry.RepoName("sample/" + h.RandString(10))
	appPath := filepath.Join("..", "..", "testdata", "mock_app.zip")
	output := pack.RunSuccessfully("build", repoName, "-p", appPath)
	assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)
}
