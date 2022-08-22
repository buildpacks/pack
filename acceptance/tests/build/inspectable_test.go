//go:build acceptance
// +build acceptance

package build_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/buildpacks/pack/acceptance/harness"
	h "github.com/buildpacks/pack/testhelpers"
)

func test_app_image_is_inspectable(t *testing.T, th *harness.BuilderTestHarness, combo harness.BuilderCombo) {
	assert := h.NewAssertionManager(t)

	registry := th.Registry()
	imageManager := th.ImageManager()
	runImageName := th.Stack().RunImage.Name
	runImageMirror := th.Stack().RunImage.MirrorName

	pack := combo.Pack()

	appPath := filepath.Join("..", "..", "testdata", "mock_app")
	repoName := registry.RepoName("some-org/" + h.RandString(10))

	output := pack.RunSuccessfully(
		"build", repoName,
		"-p", appPath,
	)

	localRunImageMirror := registry.RepoName("pack-test/run-mirror")
	imageManager.TagImage(runImageName, localRunImageMirror)
	defer imageManager.CleanupImages(localRunImageMirror)

	pack.JustRunSuccessfully("config", "run-image-mirrors", "add", runImageName, "-m", localRunImageMirror)
	defer pack.JustRunSuccessfully("config", "run-image-mirrors", "remove", runImageName)

	inspectCmd := "inspect"
	if !pack.Supports("inspect") {
		inspectCmd = "inspect-image"
	}

	var (
		webCommand      string
		helloCommand    string
		helloArgs       []string
		helloArgsPrefix string
		imageWorkdir    string
	)
	if imageManager.HostOS() == "windows" {
		webCommand = ".\\run"
		helloCommand = "cmd"
		helloArgs = []string{"/c", "echo hello world"}
		helloArgsPrefix = " "
		imageWorkdir = "c:\\workspace"
	} else {
		webCommand = "./run"
		helloCommand = "echo"
		helloArgs = []string{"hello", "world"}
		helloArgsPrefix = ""
		imageWorkdir = "/workspace"
	}

	formats := []compareFormat{
		{
			extension:   "json",
			compareFunc: assert.EqualJSON,
			outputArg:   "json",
		},
		{
			extension:   "yaml",
			compareFunc: assert.EqualYAML,
			outputArg:   "yaml",
		},
		{
			extension:   "toml",
			compareFunc: assert.EqualTOML,
			outputArg:   "toml",
		},
	}
	for _, format := range formats {
		t.Logf("inspecting image %s format", format.outputArg)

		output = pack.RunSuccessfully(inspectCmd, repoName, "--output", format.outputArg)
		expectedOutput := pack.FixtureManager().TemplateFixture(
			fmt.Sprintf("inspect_image_local_output.%s", format.extension),
			map[string]interface{}{
				"image_name":             repoName,
				"base_image_id":          h.ImageID(t, runImageMirror),
				"base_image_top_layer":   h.TopLayerDiffID(t, runImageMirror),
				"run_image_local_mirror": localRunImageMirror,
				"run_image":              runImageName,
				"run_image_mirror":       runImageMirror,
				"web_command":            webCommand,
				"hello_command":          helloCommand,
				"hello_args":             helloArgs,
				"hello_args_prefix":      helloArgsPrefix,
				"image_workdir":          imageWorkdir,
			},
		)

		format.compareFunc(output, expectedOutput)
	}
}

type compareFormat struct {
	extension   string
	compareFunc func(string, string)
	outputArg   string
}
