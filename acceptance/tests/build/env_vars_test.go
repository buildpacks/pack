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

func test_env_file(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	assert := h.NewAssertionManager(t)
	registry := th.Registry()
	imageManager := th.ImageManager()
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack := combo.Pack()

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

func test_env_vars(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	assert := h.NewAssertionManager(t)
	registry := th.Registry()
	imageManager := th.ImageManager()
	assertImage := assertions.NewImageAssertionManager(t, imageManager, &registry)

	pack := combo.Pack()

	assert.Succeeds(os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment"))
	t.Cleanup(func() {
		assert.Succeeds(os.Unsetenv("ENV2_CONTENTS"))
	})

	repoName := registry.RepoName("some-org/" + h.RandString(10))
	output := pack.RunSuccessfully(
		"build", repoName,
		"-p", filepath.Join("..", "..", "testdata", "mock_app"),
		"--env", "DETECT_ENV_BUILDPACK=true",
		"--env", `ENV1_CONTENTS="Env1 Layer Contents From Command Line"`,
		"--env", "ENV2_CONTENTS",
	)

	assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)
	assertImage.RunsWithOutput(
		repoName,
		"Env2 Layer Contents From Environment",
		"Env1 Layer Contents From Command Line",
	)
}
