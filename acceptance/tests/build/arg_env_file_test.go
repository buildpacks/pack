//go:build acceptance
// +build acceptance

package build_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_arg_env_file(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	assert := h.NewAssertionManager(t)
	registry := th.Registry()
	imageManager := th.ImageManager()
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack := combo.Pack()

	// mark builder as trusted
	// TODO: Windows fails with "Access is denied" when builder is not trusted
	pack.JustRunSuccessfully("config", "trusted-builders", "add", combo.BuilderName())
	defer pack.JustRunSuccessfully("config", "trusted-builders", "remove", combo.BuilderName())

	envfile, err := ioutil.TempFile("", "envfile")
	assert.Nil(err)
	defer envfile.Close()

	err = os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment")
	assert.Nil(err)

	_, err = envfile.WriteString(`
DETECT_ENV_BUILDPACK=true
ENV1_CONTENTS=Env1 Layer Contents From File
ENV2_CONTENTS
`)
	assert.Nil(err)
	err = envfile.Close()
	assert.Nil(err)

	envPath := envfile.Name()
	t.Cleanup(func() {
		assert.Succeeds(os.Unsetenv("ENV2_CONTENTS"))
		assert.Succeeds(os.RemoveAll(envPath))
	})

	repoName := registry.RepoName("some-org/" + h.RandString(10))
	output := pack.RunSuccessfully(
		"build", repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
		"--env-file", envPath,
	)

	assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)
	assertImage.RunsWithOutput(
		repoName,
		"Env2 Layer Contents From Environment",
		"Env1 Layer Contents From File",
	)
}
