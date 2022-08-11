//go:build acceptance
// +build acceptance

package acceptance_new

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/buildpacks"
	"github.com/buildpacks/pack/acceptance/config"
	"github.com/buildpacks/pack/acceptance/invoke"
	"github.com/buildpacks/pack/acceptance/managers"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/docker/docker/client"
)

func TestBuild(t *testing.T) {
	h.RequireDocker(t)
	rand.Seed(time.Now().UTC().UnixNano())

	assert := h.NewAssertionManager(t)

	// docker cli
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	assert.Nil(err)

	imageManager := managers.NewImageManager(t, dockerCli)

	// run temp registry
	registryConfig := h.RunRegistry(t)
	defer registryConfig.RmRegistry(t)

	// image assert
	assertImage := assertions.NewImageAssertionManager(t, imageManager, registryConfig)

	// gather config
	inputConfigManager, err := config.NewInputConfigurationManager()
	assert.Nil(err)

	assetsConfig := config.NewAssetManager(t, assert, inputConfigManager, filepath.Join("..", "testdata"))

	// create stack
	runImageName := "pack-test/run"
	buildImageName := "pack-test/build"

	runImageMirror := registryConfig.RepoName(runImageName)
	err = createStack(t, dockerCli, registryConfig, imageManager, runImageName, buildImageName, runImageMirror)
	assert.Nil(err)

	stackBaseImages := map[string][]string{
		"linux":   {"ubuntu:bionic"},
		"windows": {"mcr.microsoft.com/windows/nanoserver:1809", "golang:1.17-nanoserver-1809"},
	}
	baseStackNames := stackBaseImages[imageManager.HostOS()]
	defer imageManager.CleanupImages(baseStackNames...)
	defer imageManager.CleanupImages(runImageName, buildImageName, runImageMirror)

	for _, combo := range inputConfigManager.Combinations() {
		combo := combo

		t.Logf("Running combo: %s", combo)

		lifecycle := assetsConfig.NewLifecycleAsset(combo.Lifecycle)

		pack := invoke.NewPackInvoker(t, assert, assetsConfig.NewPackAsset(combo.Pack), registryConfig.DockerConfigDir)
		pack.EnableExperimental()
		pack.JustRunSuccessfully("config", "lifecycle-image", lifecycle.Image())

		createBuilderPack := invoke.NewPackInvoker(t, assert, assetsConfig.NewPackAsset(combo.PackCreateBuilder), registryConfig.DockerConfigDir)
		createBuilderPack.EnableExperimental()

		buildpackManager := buildpacks.NewBuildpackManager(
			t,
			assert,
			buildpacks.WithBuildpackAPIVersion(lifecycle.EarliestBuildpackAPIVersion()),
			buildpacks.WithBaseDir(filepath.Join("..", "testdata", "mock_buildpacks")),
		)

		repo := "some-org/" + h.RandString(10)
		repoName := registryConfig.RepoName(repo)

		// create builder
		builderName, err := createBuilder(t, assert, registryConfig, imageManager, dockerCli, createBuilderPack, lifecycle, buildpackManager, runImageMirror)
		assert.Nil(err)
		defer imageManager.CleanupImages(builderName)

		t.Run("creates a runnable, rebuildable image on daemon from app dir", func(t *testing.T) {
			appPath := filepath.Join("..", "testdata", "mock_app")

			output := pack.RunSuccessfully(
				"build", repoName,
				"-B", builderName,
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
			localRunImageMirror := registryConfig.RepoName("pack-test/run-mirror")
			imageManager.TagImage(runImageName, localRunImageMirror)
			defer imageManager.CleanupImages(localRunImageMirror)
			pack.JustRunSuccessfully("config", "run-image-mirrors", "add", runImageName, "-m", localRunImageMirror)

			t.Log("rebuild")
			output = pack.RunSuccessfully(
				"build", repoName,
				"-B", builderName,
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
				"-B", builderName,
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

			t.Log("inspecting image")
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
		})
	}

	// suiteManager.CleanUp()
	// imageManager.CleanupImages(repoName)

	// ref, err := name.ParseReference(repoName, name.WeakValidation)
	// assert.Nil(err)
	// cacheImage := cache.NewImageCache(ref, dockerCli)
	// buildCacheVolume := cache.NewVolumeCache(ref, cache.CacheInfo{}, "build", dockerCli)
	// launchCacheVolume := cache.NewVolumeCache(ref, cache.CacheInfo{}, "launch", dockerCli)
	// cacheImage.Clear(context.TODO())
	// buildCacheVolume.Clear(context.TODO())
	// launchCacheVolume.Clear(context.TODO())
}
