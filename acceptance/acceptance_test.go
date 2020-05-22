// +build acceptance

package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/components"
	"github.com/buildpacks/pack/acceptance/managers"
	"github.com/buildpacks/pack/internal/cache"
	"github.com/buildpacks/pack/internal/style"
	h "github.com/buildpacks/pack/testhelpers"
)

var (
	dockerCli    *client.Client
	suiteManager *managers.SuiteManager
)

func TestAcceptance(t *testing.T) {
	var err error

	h.RequireDocker(t)
	rand.Seed(time.Now().UTC().UnixNano())

	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	assert := assertions.NewAssertionManager(t, dockerCli)
	assert.Nil(err)

	registryConfig := h.RunRegistry(t)
	defer registryConfig.StopRegistry(t)

	inputConfigManager, err := managers.NewInputConfigurationManager()
	assert.Nil(err)

	assetsConfig, err := managers.ConvergedAssetManager(t, inputConfigManager)
	assert.Nil(err)

	suiteManager = managers.NewSuiteManager(t.Logf)
	suite := spec.New("acceptance suite", spec.Report(report.Terminal{}))

	if inputConfigManager.Combinations().IncludesCurrentSubjectPack() {
		suite("p_current", func(t *testing.T, when spec.G, it spec.S) {
			testWithoutSpecificBuilderRequirement(
				t,
				when,
				it,
				assetsConfig,
				registryConfig,
			)
		}, spec.Report(report.Terminal{}))
	}

	for _, combo := range inputConfigManager.Combinations() {
		t.Logf(`setting up run combination %s: %s`,
			style.Symbol(combo.String()),
			combo.Describe(assetsConfig),
		)

		comboCopy := combo

		suite(combo.String(), func(t *testing.T, when spec.G, it spec.S) {
			testAcceptance(
				t,
				when,
				it,
				comboCopy,
				assetsConfig,
				registryConfig,
			)
		}, spec.Report(report.Terminal{}))
	}

	suite.Run(t)

	assert.Nil(suiteManager.CleanUp())
}

// These tests either (a) do not require a builder or (b) do not require a specific builder to be provided
// in order to test compatibility.
// They should only be run against the "current" (i.e., master) version of pack.
func testWithoutSpecificBuilderRequirement(
	t *testing.T,
	when spec.G,
	it spec.S,
	assetManager managers.AssetManager,
	registryConfig *h.TestRegistryConfig,
) {
	var (
		pack   *components.PackExecutor
		assert = assertions.NewAssertionManager(t, dockerCli)
	)

	it.Before(func() {
		pack = managers.NewPackExecutor(t, assetManager, managers.Current, registryConfig, assert)
	})

	it.After(func() {
		pack.Cleanup()
	})

	when("invalid subcommand", func() {
		it("prints usage", func() {
			output, err := pack.RunWithCombinedOutput("some-bad-command")
			assert.NotNil(err)

			assert.Contains(output, `unknown command "some-bad-command" for "pack"`)
			assert.Contains(output, `Run 'pack --help' for usage.`)
		})
	})

	when("suggest-builders", func() {
		it("displays suggested builders", func() {
			output := pack.SuccessfulRunWithOutput("suggest-builders")

			assertOutput := assert.NewOutputAssertionManager(output)
			assertOutput.IncludesSuggestedBuildersHeading()
			assertOutput.IncludesGoogleBuilder()
			assertOutput.IncludesHerokuBuilder()
			assertOutput.IncludesPaketoBuilders()
		})
	})

	when("suggest-stacks", func() {
		it("displays suggested stacks", func() {
			output := pack.SuccessfulRunWithOutput("suggest-stacks")

			assert.NewOutputAssertionManager(output).IncludesSuggestedStacksHeading()
		})
	})

	when("set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			builderName := "gcr.io/paketo-buildpacks/builder:base"

			output := pack.SuccessfulRunWithOutput("set-default-builder", builderName)

			assert.NewOutputAssertionManager(output).ReportsSettingDefaultBuilder(builderName)
		})
	})

	when("trust-builder", func() {
		it("sets the builder as trusted in ~/.pack/config.toml", func() {
			h.SkipIf(t, pack.Supports("trust-builder"), "pack does not support 'trust-builder'")
			builderName := "some-builder" + h.RandString(10)

			pack.SuccessfulRun("trust-builder", builderName)

			packConfigFileContents := pack.FixtureMustExist("config.toml")
			assert.Contains(packConfigFileContents, builderName)
		})
	})

	when("package-buildpack", func() {
		var (
			tmpDir                  string
			simplePackageConfigPath string
			containerManager        managers.TestContainerManager
		)

		it.Before(func() {
			h.SkipIf(t,
				!pack.Supports("package-buildpack"),
				"pack does not support 'package-buildpack'",
			)

			containerManager = managers.NewTestContainerManager(t, assert, dockerCli)

			h.SkipIf(t, containerManager.HostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

			var err error
			tmpDir, err = ioutil.TempDir("", "package-buildpack-tests")
			assert.Nil(err)

			simplePackageConfigPath = filepath.Join(tmpDir, "package.toml")
			h.CopyFile(t, pack.FixtureMustExist("package.toml"), simplePackageConfigPath)

			buildpackManager := managers.NewBuildpackManager(t, assert)

			buildpackManager.PlaceBuildpacksInDir(
				tmpDir,
				components.SimpleLayersParentBuildpack,
				components.SimpleLayersBuildpack,
			)
		})

		it.After(func() {
			assert.Nil(os.RemoveAll(tmpDir))
		})

		when("no --format is provided", func() {
			it("creates the package as image", func() {
				packageName := "test/package-" + h.RandString(10)

				output := pack.SuccessfulRunWithOutput(
					"package-buildpack", packageName,
					"-p", simplePackageConfigPath,
				)

				assert.Contains(output, fmt.Sprintf("Successfully created package '%s'", packageName))
				defer containerManager.RemoveImages(packageName)

				assert.ImageExistsLocally(packageName)
			})

			it("creates the package as a local image", func() {
				t.Log("package w/ only buildpacks")
				nestedPackageName := newPackageName()
				assert.PackageCreated(
					nestedPackageName,
					pack.PackageBuildpack(nestedPackageName, simplePackageConfigPath),
				)
				defer containerManager.RemoveImages(nestedPackageName)

				assert.ImageExistsLocally(nestedPackageName)

				aggregatePackageConfigFile := pack.AggregatePackageFixture(
					nestedPackageName,
					"simple-layers-parent-buildpack.tgz",
					tmpDir,
				)

				packageName := newPackageName()
				assert.PackageCreated(packageName, pack.PackageBuildpack(packageName, aggregatePackageConfigFile.Name()))

				defer containerManager.RemoveImages(packageName)
				assert.ImageExistsLocally(packageName)
			})

			when("--publish", func() {
				it("publishes image to registry", func() {
					nestedPackageName := registryConfig.RepoName(newPackageName())
					assert.PackagePublished(
						nestedPackageName,
						pack.PackageBuildpack(nestedPackageName, simplePackageConfigPath, "--publish"),
					)
					defer containerManager.RemoveImages(nestedPackageName)

					aggregatePackageConfigFile := pack.AggregatePackageFixture(
						nestedPackageName,
						"simple-layers-parent-buildpack.tgz",
						tmpDir,
					)

					packageName := registryConfig.RepoName(newPackageName())
					assert.PackagePublished(
						packageName,
						pack.PackageBuildpack(packageName, aggregatePackageConfigFile.Name(), "--publish"),
					)

					defer containerManager.RemoveImages(packageName)

					assert.ImageExistsOnlyInRegistry(packageName, registryConfig)
				})
			})

			when("--no-pull", func() {
				// TODO: Is this test important? The flow is identical to without --no-pull
				it("should use local image", func() {
					nestedPackageName := newPackageName()
					assert.PackageCreated(
						nestedPackageName,
						pack.PackageBuildpack(nestedPackageName, simplePackageConfigPath),
					)
					defer containerManager.RemoveImages(nestedPackageName)

					aggregatePackageConfigFile := pack.AggregatePackageFixture(
						nestedPackageName,
						"simple-layers-parent-buildpack.tgz",
						tmpDir,
					)

					packageName := registryConfig.RepoName(newPackageName())
					assert.PackageCreated(
						packageName,
						pack.PackageBuildpack(packageName, aggregatePackageConfigFile.Name(), "--no-pull"),
					)

					defer containerManager.RemoveImages(packageName)

					assert.ImageExistsLocally(packageName)
				})

				it("should not pull image from registry", func() {
					nestedPackageName := registryConfig.RepoName(newPackageName())
					assert.PackagePublished(
						nestedPackageName,
						pack.PackageBuildpack(nestedPackageName, simplePackageConfigPath, "--publish"),
					)
					defer containerManager.RemoveImages(nestedPackageName)

					aggregatePackageConfigFile := pack.AggregatePackageFixture(
						nestedPackageName,
						"simple-layers-parent-buildpack.tgz",
						tmpDir,
					)

					packageName := registryConfig.RepoName(newPackageName())
					defer containerManager.RemoveImages(packageName)

					output, err := pack.RunWithCombinedOutput(
						"package-buildpack", packageName,
						"-p", aggregatePackageConfigFile.Name(),
						"--no-pull",
					)
					assert.Error(err)
					assert.Contains(output, fmt.Sprintf("image '%s' does not exist on the daemon", nestedPackageName))
				})
			})
		})

		when("--format file", func() {
			it.Before(func() {
				h.SkipIf(t, !pack.Supports("package-buildpack --format"), "format not supported")
			})

			it("creates the package", func() {
				outputFile := filepath.Join(tmpDir, "package.cnb")

				assert.PackageCreated(
					outputFile,
					pack.PackageBuildpack(outputFile, filepath.Join(tmpDir, "package.toml"), "--format", "file"),
				)

				h.AssertTarball(t, outputFile)
			})
		})

		when("package.toml is invalid", func() {
			it("displays an error", func() {
				output, err := pack.RunWithCombinedOutput(
					"package-buildpack", "some-pack",
					"-p", pack.FixtureMustExist("invalid_package.toml"),
				)

				assert.Error(err)
				assert.Contains(output, "reading config")
			})
		})
	})

	when("report", func() {
		it.Before(func() {
			h.SkipIf(t, !pack.Supports("report"), "pack does not support 'report' command")
		})

		when("default builder is set", func() {
			it("outputs information", func() {
				pack.SuccessfulRun("set-default-builder", "gcr.io/paketo-buildpacks/builder:base")

				output := pack.SuccessfulRunWithOutput("report")

				version := pack.Version()
				expectedOutput := pack.TemplateFixture(
					"report_output.txt",
					map[string]interface{}{
						"DefaultBuilder": "gcr.io/paketo-buildpacks/builder:base",
						"Version":        version,
						"OS":             runtime.GOOS,
						"Arch":           runtime.GOARCH,
					},
				)

				assert.Equal(output, expectedOutput)
			})
		})
	})

	when("build", func() {
		when("default builder is not set", func() {
			it("informs the user", func() {
				output, err := pack.RunWithCombinedOutput(
					"build", "some/image",
					"-p", filepath.Join("testdata", "mock_app"),
				)
				assert.NotNil(err)

				assertOutput := assert.NewOutputAssertionManager(output)

				assertOutput.IncludesMessageToSetDefaultBuilder()
				assertOutput.IncludesGoogleBuilder()
				assertOutput.IncludesHerokuBuilder()
				assertOutput.IncludesPaketoBuilders()
			})
		})
	})
}

func testAcceptance(
	t *testing.T,
	when spec.G,
	it spec.S,
	combo *managers.RunCombo,
	assetManager managers.AssetManager,
	registry *h.TestRegistryConfig,
) {

	var (
		pack              *components.PackExecutor
		createBuilderPack *components.PackExecutor
		stackManager      managers.StackManager
		testLifecycle     *components.TestLifecycle
		packageManager    managers.PackageManager
		builderManager    managers.BuilderManager
		containerManager  managers.TestContainerManager
		buildpackManager  managers.BuildpackManager

		assert = assertions.NewAssertionManager(t, dockerCli)
	)

	it.Before(func() {
		pack = managers.NewPackExecutor(t, assetManager, combo.Pack, registry, assert)
		createBuilderPack = managers.NewPackExecutor(t, assetManager, combo.PackCreateBuilder, registry, assert)

		containerManager = managers.NewTestContainerManager(t, assert, dockerCli)

		stackManager = managers.NewStackManager(t, dockerCli, registry, assert, containerManager)
		suiteManager.RegisterCleanUp("cleanup-stack-images", stackManager.Cleanup)
		testLifecycle = managers.NewTestLifecycle(assetManager, combo.Lifecycle)

		packageManager = managers.NewPackageManager(t, assert, suiteManager, createBuilderPack)
		suiteManager.RegisterCleanUp("cleanup-package-manager", packageManager.Cleanup)

		builderManager = managers.NewBuilderManager(
			t,
			assert,
			registry,
			dockerCli,
			createBuilderPack,
			testLifecycle,
			packageManager,
			stackManager,
			combo,
		)

		buildpackManager = managers.NewBuildpackManager(t, assert)

		suiteManager.RegisterCleanUp(
			fmt.Sprintf("cleanup-builder-%s", builderManager.BuilderComboDescription()),
			builderManager.EnsureComboBuilderCleanedUp,
		)
	})

	it.After(func() {
		pack.Cleanup()
	})

	when("create-builder invoked with invalid builder.toml", func() {
		it("displays an error", func() {
			h.SkipIf(t,
				!createBuilderPack.SupportsFeature(components.BuilderTomlValidation),
				"builder.toml validation not supported",
			)

			builderConfigPath := createBuilderPack.FixtureMustExist("invalid_builder.toml")

			output, err := createBuilderPack.RunWithCombinedOutput(
				"create-builder", "some-builder:build",
				"--builder-config", builderConfigPath,
			)
			assert.NotNil(err)
			assert.Contains(output, "invalid builder toml")
		})
	})

	when("creating a windows builder", func() {
		var tmpDir string

		it.Before(func() {
			h.SkipIf(t, containerManager.HostOS() != "windows", "The current Docker daemon does not support Windows-based containers")

			var err error
			tmpDir, err = ioutil.TempDir("", "create-test-builder")
			assert.Nil(err)
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

		when("experimental is disabled", func() {
			it("fails", func() {
				stackManager.EnsureDefaultStackCreated()

				buildpack := components.NoOpBuildpack
				buildpackManager.PlaceBuildpacksInDir(tmpDir, buildpack)
				builderConfigPath := builderManager.NewDynamicBuilderConfig(buildpack).ConfigFile(tmpDir)

				output, err := createBuilderPack.RunWithCombinedOutput(
					"create-builder",
					"--no-color",
					registry.RepoName(fmt.Sprintf("test/builder-%s", h.RandString(8))),
					"-b", builderConfigPath,
				)

				assert.NotNil(err)
				assert.NewOutputAssertionManager(output).ReportsWindowsContainersExperimental()
			})
		})

		when("experimental is enabled", func() {
			it("succeeds", func() {
				stackManager.EnsureDefaultStackCreated()

				createBuilderPack.EnableExperimental()

				testBuilder := builderManager.CreateWindowsBuilder()
				defer testBuilder.Cleanup()

				assert.ImageIsWindows(testBuilder.Name())
			})
		})
	})

	when("build", func() {
		var (
			assertAppImage          assertions.ImageAssertionManager
			appName, appImageName   string
			mockAppPath             = filepath.Join("testdata", "mock_app")
			mixedComponentMessenger components.MixedComponents
		)

		it.Before(func() {
			h.SkipIf(t, containerManager.HostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

			appName = fmt.Sprintf("some-org/%s", h.RandString(10))
			appImageName = registry.RepoName(appName)

			assertAppImage = assert.NewImageAssertionManager(appImageName, containerManager)

			mixedComponentMessenger = components.NewMixedComponents(t, testLifecycle, pack)
		})

		it.After(func() {
			h.DockerRmi(dockerCli, appImageName)
			ref, err := name.ParseReference(appImageName, name.WeakValidation)
			assert.Nil(err)
			cacheImage := cache.NewImageCache(ref, dockerCli)
			buildCacheVolume := cache.NewVolumeCache(ref, "build", dockerCli)
			launchCacheVolume := cache.NewVolumeCache(ref, "launch", dockerCli)
			cacheImage.Clear(context.TODO())
			buildCacheVolume.Clear(context.TODO())
			launchCacheVolume.Clear(context.TODO())
		})

		it("creates a runnable, rebuildable image on daemon from app dir", func() {
			stackManager.EnsureDefaultStackCreated()
			testBuilder := builderManager.EnsureComboBuilderExists()
			pack.SuccessfulRun("set-default-builder", testBuilder.Name())
			if pack.Supports("trust-builder") {
				pack.SuccessfulRun("trust-builder", testBuilder.Name())
			}

			appPath := filepath.Join("testdata", "mock_app")
			output := pack.SuccessfulRunWithOutput("build", appImageName, "-p", appPath)
			cleanupAppImageTask := containerManager.CleanupTaskForImageByName(appImageName)
			defer cleanupAppImageTask()

			assertOutput := assert.NewOutputAssertionManager(output)

			assertOutput.ReportsSuccessfulImageBuild(appImageName)
			assertOutput.ReportsUsingBuildCacheVolume()
			assertOutput.ReportsSelectingRunImageMirror(stackManager.RunImageMirror())

			assertAppImage.RunsMockAppWithOutput("Launch Dep Contents", "Cached Dep Contents")
			assertAppImage.HasBase(managers.RunImage)
			assertAppImage.HasRunImageMetadata(managers.RunImage, stackManager.RunImageMirror())

			assert.NoImageExistsInRegistry(appName, registry)

			t.Log("add a local mirror")
			localRunImageMirror := registry.RepoName("pack-test/run-mirror")
			assert.Nil(dockerCli.ImageTag(context.Background(), managers.RunImage, localRunImageMirror))
			defer containerManager.RemoveImages(localRunImageMirror)
			pack.SuccessfulRun("set-run-image-mirrors", managers.RunImage, "-m", localRunImageMirror)

			t.Log("rebuild")
			output = pack.SuccessfulRunWithOutput("build", appImageName, "-p", appPath)
			cleanupRebuiltAppImageTask := containerManager.CleanupTaskForImageByName(appImageName)
			defer cleanupRebuiltAppImageTask()

			assertOutput = assert.NewOutputAssertionManager(output)
			assertOutput.ReportsSuccessfulImageBuild(appImageName)
			assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirror)

			assertMixedOutput := assert.NewMixedComponentOutputAssertionManager(output, mixedComponentMessenger)
			cachedLaunchLayer := "simple/layers:cached-launch-layer"
			assertMixedOutput.ReportsRestoresCachedLayer(cachedLaunchLayer)
			assertMixedOutput.ReportsExporterReusesUnchangedLayer(cachedLaunchLayer)
			assertMixedOutput.ReportsCacheReuse(cachedLaunchLayer)

			assertAppImage.RunsMockAppWithOutput("Launch Dep Contents", "Cached Dep Contents")

			t.Log("rebuild with --clear-cache")
			output = pack.SuccessfulRunWithOutput("build", appImageName, "-p", appPath, "--clear-cache")

			assert.NewOutputAssertionManager(output).ReportsSuccessfulImageBuild(appImageName)

			assertMixedOutput = assert.NewMixedComponentOutputAssertionManager(output, mixedComponentMessenger)
			assertMixedOutput.ReportsSkippingRestore()
			assertMixedOutput.ReportsSkippingBuildpackLayerAnalysis()
			assertMixedOutput.ReportsExporterReusesUnchangedLayer(cachedLaunchLayer)
			assertMixedOutput.ReportsCacheCreation(cachedLaunchLayer)

			if pack.Supports("inspect-image") {
				t.Log("validating inspect-image output")
				output := pack.SuccessfulRunWithOutput("inspect-image", appImageName)

				expectedOutput := pack.TemplateFixture(
					"inspect_image_local_output.txt",
					map[string]interface{}{
						"image_name":             appImageName,
						"base_image_id":          containerManager.ImageIDForReference(stackManager.RunImageMirror()),
						"base_image_top_layer":   containerManager.TopLayerDiffID(stackManager.RunImageMirror()),
						"run_image_local_mirror": localRunImageMirror,
						"run_image_mirror":       stackManager.RunImageMirror(),
						"show_reference":         testLifecycle.ShouldShowReference(),
						"show_processes":         testLifecycle.ShouldShowProcesses(),
					},
				)

				assert.Equal(output, expectedOutput)
			}
		})

		it("supports building app from a zip file", func() {
			stackManager.EnsureDefaultStackCreated()
			testBuilder := builderManager.EnsureComboBuilderExists()
			pack.SetDefaultTrustedBuilder(testBuilder.Name())

			appPath := filepath.Join("testdata", "mock_app.zip")
			output := pack.SuccessfulRunWithOutput("build", appImageName, "-p", appPath)

			assert.NewOutputAssertionManager(output).ReportsSuccessfulImageBuild(appImageName)

			appImageCleanupTask := containerManager.CleanupTaskForImageByName(appImageName)
			defer appImageCleanupTask()
		})

		when("builder is untrusted", func() {
			it("uses the 5 phases", func() {
				stackManager.EnsureDefaultStackCreated()
				untrustedBuilder := builderManager.CreateOneOffBuilder()
				defer containerManager.RemoveImages(untrustedBuilder.Name())

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"-B", untrustedBuilder.Name(),
				)

				// TODO: This test could be more explicit by ensuring the correct phase names are included with each message
				assert.ContainsAll(output, "[detector]", "[analyzer]", "[builder]", "[exporter]")
				assert.NewOutputAssertionManager(output).ReportsSuccessfulImageBuild(appImageName)
			})
		})

		when("--network", func() {
			var (
				tmpDir string
			)

			it.Before(func() {
				h.SkipIf(
					t,
					!pack.Supports("build --network"),
					"--network flag not supported for build",
				)

				var err error
				tmpDir, err = ioutil.TempDir("", "archive-buildpacks-")
				assert.Nil(err)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			when("the network mode is not provided", func() {
				it("reports that build and detect are online", func() {
					stackManager.EnsureDefaultStackCreated()
					testBuilder := builderManager.EnsureComboBuilderExists()
					pack.SetDefaultTrustedBuilder(testBuilder.Name())

					buildpackManager.PlaceBuildpacksInDir(tmpDir, components.InternetCapableBuildpack)

					output := pack.SuccessfulRunWithOutput(
						"build", appImageName,
						"-p", mockAppPath,
						"--buildpack", filepath.Join(tmpDir, components.InternetCapableBuildpack.FileName()),
					)

					assert.NewMixedComponentOutputAssertionManager(output, mixedComponentMessenger).ReportsConnectedToInternet()
				})
			})

			when("the network mode is set to default", func() {
				it("reports that build and detect are online", func() {
					stackManager.EnsureDefaultStackCreated()
					testBuilder := builderManager.EnsureComboBuilderExists()
					pack.SetDefaultTrustedBuilder(testBuilder.Name())

					buildpackManager.PlaceBuildpacksInDir(tmpDir, components.InternetCapableBuildpack)

					output := pack.SuccessfulRunWithOutput(
						"build", appImageName,
						"-p", mockAppPath,
						"--buildpack", filepath.Join(tmpDir, components.InternetCapableBuildpack.FileName()),
						"--network", "default",
					)

					assert.NewMixedComponentOutputAssertionManager(output, mixedComponentMessenger).ReportsConnectedToInternet()
				})
			})

			when("the network mode is set to none", func() {
				it("reports that build and detect are offline", func() {
					stackManager.EnsureDefaultStackCreated()
					testBuilder := builderManager.EnsureComboBuilderExists()
					pack.SetDefaultTrustedBuilder(testBuilder.Name())

					buildpackManager.PlaceBuildpacksInDir(tmpDir, components.InternetCapableBuildpack)

					output := pack.SuccessfulRunWithOutput(
						"build", appImageName,
						"-p", mockAppPath,
						"--buildpack", filepath.Join(tmpDir, components.InternetCapableBuildpack.FileName()),
						"--network", "none",
					)

					assert.NewMixedComponentOutputAssertionManager(output, mixedComponentMessenger).ReportsDisconnectedFromInternet()
				})
			})
		})

		when("--volume", func() {
			var tmpDir string

			it.Before(func() {
				h.SkipIf(t,
					!pack.SupportsFeature(components.CustomVolumeMounts),
					"pack 0.11.0 shipped with a volume mounting bug",
				)

				var err error
				tmpDir, err = ioutil.TempDir("", "volume-buildpack-tests-")
				assert.Nil(err)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			it.Focus("mounts the provided volume in the detect and build phases", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				tempVolume, err := ioutil.TempDir(tmpDir, "my-volume-mount-source")
				assert.Nil(err)
				assert.Nil(os.Chmod(tempVolume, 0755)) // Override umask

				// Some OSes (like macOS) use symlinks for the standard temp dir.
				// Resolve it so it can be properly mounted by the Docker daemon.
				tempVolume, err = filepath.EvalSymlinks(tempVolume)
				assert.Nil(err)

				fileName := "some-file"
				fileContents := "some-string\n"
				mountTarget := "/my-volume-mount-target"

				err = ioutil.WriteFile(filepath.Join(tempVolume, fileName), []byte(fileContents), 0755)
				assert.Nil(err)

				buildpackManager.PlaceBuildpacksInDir(tmpDir, components.VolumeBuildpack)

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", filepath.Join(tmpDir, components.VolumeBuildpack.FileName()),
					"--volume", fmt.Sprintf("%s:%s", tempVolume, mountTarget),
				)

				assert.NewOutputAssertionManager(output).ReportsReadingFileContents(
					fmt.Sprintf("%s/%s", mountTarget, fileName),
					fileContents,
					pack,
				)
			})
		})

		when("--default-process", func() {
			it("sets the default process from those in the process list", func() {
				h.SkipIf(
					t,
					!pack.Supports("build --default-process"),
					"--default-process flag is not supported",
				)

				h.SkipIf(t,
					!testLifecycle.SupportsFeature(components.DefaultProcess),
					"skipping default process. Lifecycle does not support it",
				)

				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				pack.SuccessfulRun(
					"build", appImageName,
					"--default-process", "hello",
					"-p", mockAppPath,
				)

				assertAppImage.RunsMockAppWithLogs("hello world")
			})
		})

		when("--buildpack is an ID", func() {
			it("adds the buildpacks to the builder if necessary and runs them", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", "simple/layers", // Can omit version if only one
					"--buildpack", "noop.buildpack@noop.buildpack.version",
				)

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsBuildStep("Simple Layers Buildpack")
				assertOutput.ReportsBuildStep("NOOP Buildpack")
				assertOutput.ReportsSuccessfulImageBuild(appImageName)

				assertAppImage.RunsMockAppWithOutput("Launch Dep Contents", "Cached Dep Contents")
			})
		})

		when("--buildpack is an archive", func() {
			var tmpDir string

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "archive-buildpack-tests-")
				assert.Nil(err)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			it("adds the buildpack to the builder and runs it", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				buildpackManager.PlaceBuildpacksInDir(tmpDir, components.NotInBuilderBuildpack)

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", filepath.Join(tmpDir, components.NotInBuilderBuildpack.FileName()),
				)

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsAddingBuildpack("local/bp", "local-bp-version")
				assertOutput.ReportsBuildStep("Local Buildpack")
				assertOutput.ReportsSuccessfulImageBuild(appImageName)
			})
		})

		when("--buildpack is directory", func() {
			it("adds the buildpacks to the builder and runs it", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "buildpack directories not supported on windows")

				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", buildpackManager.FolderBuildpackPath("not-in-builder-buildpack"),
				)

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsAddingBuildpack("local/bp", "local-bp-version")
				assertOutput.ReportsBuildStep("Local Buildpack")
				assertOutput.ReportsSuccessfulImageBuild(appImageName)
			})
		})

		when("--buildpack is a buildpackage image", func() {
			it("adds the buildpacks to the builder and runs them", func() {
				h.SkipIf(t,
					!pack.Supports("package-buildpack"),
					"--buildpack does not accept buildpackage unless package-buildpack is supported",
				)

				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				packageImageName := packageManager.PackageBuildpack(
					components.NewPackageImageConfig(registry, dockerCli),
					"package_for_build_cmd.toml",
					[]components.TestBuildpack{
						components.SimpleLayersParentBuildpack,
						components.SimpleLayersBuildpack,
					},
				)

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", packageImageName,
				)

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsAddingBuildpack("simple/layers/parent", "simple-layers-parent-version")
				assertOutput.ReportsAddingBuildpack("simple/layers", "simple-layers-version")
				assertOutput.ReportsBuildStep("Simple Layers Buildpack")
				assertOutput.ReportsSuccessfulImageBuild(appImageName)
			})
		})

		when("--buildpack is a buildpackage file", func() {
			var tmpDir string

			it.Before(func() {
				h.SkipIf(t,
					!pack.Supports("package-buildpack --format"),
					"--buildpack does not accept buildpackage file unless package-buildpack with --format is supported",
				)

				var err error
				tmpDir, err = ioutil.TempDir("", "package-file")
				assert.Nil(err)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			it("adds the buildpacks to the builder and runs them", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				packageFileName := packageManager.PackageBuildpack(
					components.NewPackageFileConfig(),
					"package_for_build_cmd.toml",
					[]components.TestBuildpack{
						components.SimpleLayersParentBuildpack,
						components.SimpleLayersBuildpack,
					},
				)

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", packageFileName,
				)

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsAddingBuildpack("simple/layers/parent", "simple-layers-parent-version")
				assertOutput.ReportsAddingBuildpack("simple/layers", "simple-layers-version")
				assertOutput.ReportsBuildStep("Simple Layers Buildpack")
				assertOutput.ReportsSuccessfulImageBuild(appImageName)
			})
		})

		when("the buildpack stack doesn't match the builder", func() {
			var tmpDir string

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "unmatched-stack-buildpack-tests-")
				assert.Nil(err)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			it("errors", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				buildpackManager.PlaceBuildpacksInDir(tmpDir, components.OtherStackBuildpack)

				output, err := pack.RunWithCombinedOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--buildpack", filepath.Join(tmpDir, components.OtherStackBuildpack.FileName()),
				)
				assert.NotNil(err)

				assert.Contains(output, "other/stack/bp")
				assert.Contains(output, "other-stack-version")
				assert.Contains(output, "does not support stack 'pack.test.stack'")
			})
		})

		when("--env-file", func() {
			var envPath string

			it.Before(func() {
				envFile, err := ioutil.TempFile("", "envfile")
				assert.Nil(err)
				defer envFile.Close()

				err = os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment")
				assert.Nil(err)
				envFile.WriteString(`
DETECT_ENV_BUILDPACK="true"
ENV1_CONTENTS="Env1 Layer Contents From File"
ENV2_CONTENTS
`)
				envPath = envFile.Name()
			})

			it.After(func() {
				assert.Nil(os.Unsetenv("ENV2_CONTENTS"))
				assert.Nil(os.RemoveAll(envPath))
			})

			it("provides the env vars to the build and detect steps", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--env-file", envPath,
				)

				assert.NewOutputAssertionManager(output).ReportsSuccessfulImageBuild(appImageName)
				assertAppImage.RunsMockAppWithOutput(
					"Env2 Layer Contents From Environment",
					"Env1 Layer Contents From File",
				)
			})
		})

		when("--env", func() {
			it.Before(func() {
				h.AssertNil(t,
					os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment"),
				)
			})

			it.After(func() {
				assert.Nil(os.Unsetenv("ENV2_CONTENTS"))
			})

			it("provides the env vars to the build and detect steps", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--env", "DETECT_ENV_BUILDPACK=true",
					"--env", `ENV1_CONTENTS="Env1 Layer Contents From Command Line"`,
					"--env", "ENV2_CONTENTS",
				)

				assert.NewOutputAssertionManager(output).ReportsSuccessfulImageBuild(appImageName)
				assertAppImage.RunsMockAppWithOutput(
					"Env2 Layer Contents From Environment",
					"Env1 Layer Contents From Command Line",
				)
			})
		})

		when("--run-image has the correct stack ID", func() {
			var runImageName string

			it.After(func() {
				h.DockerRmi(dockerCli, runImageName)
			})

			it("uses the run image as the base image", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				runImageName = containerManager.CreateCustomRunImageOnRemote(registry)

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--run-image", runImageName,
				)

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsSuccessfulImageBuild(appImageName)
				assertOutput.ReportsPullingImage(runImageName)

				assertAppImage.RunsMockAppWithOutput("Launch Dep Contents", "Cached Dep Contents")
				assertAppImage.HasBase(runImageName)
			})
		})

		when("the run image has the wrong stack ID", func() {
			var runImageName string

			it.After(func() {
				h.DockerRmi(dockerCli, runImageName)
			})

			it("fails with a message", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				runImageName = containerManager.CreateDifferentStackRunImageOnRemote(registry)

				output, err := pack.RunWithCombinedOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--run-image", runImageName,
				)

				assert.NotNil(err)
				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsRunImageStackNotMatchingBuilder("other.stack.id", "pack.test.stack")
			})
		})

		when("--publish", func() {
			it("creates image on the registry", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", mockAppPath,
					"--publish",
					"--network", "host",
				)

				assert.NewOutputAssertionManager(output).ReportsSuccessfulImageBuild(appImageName)
				assert.AppExistsInCatalog(appName, registry)
				assert.ImageExistsInRegistry(appImageName, registry)
				defer containerManager.RemoveImages(appImageName)

				assertAppImage.RunsMockAppWithOutput("Launch Dep Contents", "Cached Dep Contents")

				if pack.Supports("inspect-image") {
					t.Log("validating inspect-image output")
					output := pack.SuccessfulRunWithOutput("inspect-image", appImageName)

					runImageMirror := stackManager.RunImageMirror()
					expectedOutput := pack.TemplateFixture(
						"inspect_image_published_output.txt",
						map[string]interface{}{
							"image_name":           appImageName,
							"base_image_ref":       strings.Join([]string{runImageMirror, h.Digest(t, runImageMirror)}, "@"),
							"base_image_top_layer": containerManager.TopLayerDiffID(runImageMirror),
							"run_image_mirror":     runImageMirror,
							"show_reference":       testLifecycle.ShouldShowReference(),
							"show_processes":       testLifecycle.ShouldShowProcesses(),
						},
					)

					assert.Equal(output, expectedOutput)
				}
			})
		})

		when("ctrl+c", func() {
			it("stops the execution", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				var buf = new(bytes.Buffer)
				command := pack.StartWithWriter(buf, "build", appImageName, "-p", mockAppPath)

				go command.TerminateAtStep("DETECTING")

				err := command.Wait()
				assert.NotNil(err)
				assert.NotContain(buf.String(), "Successfully built image")
			})
		})

		when("--descriptor has exclude", func() {
			var tmpDir string

			it.Before(func() {
				h.SkipIf(t,
					!pack.SupportsFeature(components.ExcludeAndIncludeDescriptor),
					"pack --descriptor does NOT support 'exclude' and 'include' feature",
				)

				var err error
				tmpDir, err = ioutil.TempDir("", "build-descriptor-tests")
				assert.Nil(err)

				buildpackManager.PlaceBuildpacksInDir(tmpDir, components.DescriptorBuildpack)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			it("should exclude ALL specified files and directories", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				app := components.NewTestApplication(t, assert, tmpDir, components.TestExcludeDescriptor)
				app.Create()
				app.AddTestExcludeDescriptor()

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", app.Path(),
					"--buildpack", filepath.Join(tmpDir, components.DescriptorBuildpack.FileName()),
					"--descriptor", app.DescriptorPath(),
				)
				assert.NotContain(output, "api_keys.json")
				assert.NotContain(output, "user_token")
				assert.NotContain(output, "test.sh")

				assert.Contains(output, "cookie.jar")
				assert.Contains(output, "mountain.jpg")
				assert.Contains(output, "person.png")
			})
		})

		when("--descriptor has include", func() {
			var tmpDir string

			it.Before(func() {
				h.SkipIf(t,
					!pack.SupportsFeature(components.ExcludeAndIncludeDescriptor),
					"pack --descriptor does NOT support 'exclude' and 'include' feature",
				)

				var err error
				tmpDir, err = ioutil.TempDir("", "build-descriptor-tests")
				assert.Nil(err)

				buildpackManager.PlaceBuildpacksInDir(tmpDir, components.DescriptorBuildpack)
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			it("should ONLY include specified files and directories", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				app := components.NewTestApplication(t, assert, tmpDir, components.TestIncludeDescriptor)
				app.Create()
				app.AddTestExcludeDescriptor()

				output := pack.SuccessfulRunWithOutput(
					"build", appImageName,
					"-p", app.Path(),
					"--buildpack", filepath.Join(tmpDir, components.DescriptorBuildpack.FileName()),
					"--descriptor", app.DescriptorPath(),
				)

				assert.NotContain(output, "api_keys.json")
				assert.NotContain(output, "user_token")
				assert.NotContain(output, "test.sh")

				assert.Contains(output, "cookie.jar")
				assert.Contains(output, "mountain.jpg")
				assert.Contains(output, "person.png")
			})
		})
	})

	when("inspect-builder", func() {
		it("displays configuration for a builder (local and remote)", func() {
			configuredRunImage := "some-registry.com/pack-test/run1"
			output := pack.SuccessfulRunWithOutput("set-run-image-mirrors", "pack-test/run", "--mirror", configuredRunImage)
			assert.Equal(
				output,
				"Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n",
			)

			stackManager.EnsureDefaultStackCreated()
			testBuilder := builderManager.EnsureComboBuilderExists()

			output = pack.SuccessfulRunWithOutput("inspect-builder", testBuilder.Name())

			expectedOutput := pack.TemplateVersionedFixture(
				"inspect_%s_builder_output.txt",
				createBuilderPack.Version(),
				"inspect_builder_output.txt",
				map[string]interface{}{
					"builder_name":          testBuilder.Name(),
					"lifecycle_version":     testLifecycle.Version(),
					"buildpack_api_version": testLifecycle.BuildpackAPIVersion(),
					"platform_api_version":  testLifecycle.PlatformAPIVersion(),
					"run_image_mirror":      stackManager.RunImageMirror(),
					"pack_version":          createBuilderPack.Version(),
				},
			)

			assert.Equal(output, expectedOutput)
		})
	})

	when("rebase", func() {
		const (
			initialContents1 = "contents-before-1"
			initialContents2 = "contents-before-2"
			rebasedContents1 = "contents-after-1"
			rebasedContents2 = "contents-after-2"
		)

		var (
			appName, appImageName, initialRunName, initialRunImageName, rebasedRunName, rebasedRunImageName string

			assertAppImage assertions.ImageAssertionManager
			mockAppPath    = filepath.Join("testdata", "mock_app")
		)

		it.Before(func() {
			appName = fmt.Sprintf("some-org/%s", h.RandString(10))
			appImageName = registry.RepoName(appName)

			initialRunName = fmt.Sprintf("run-before/%s", h.RandString(10))
			initialRunImageName = registry.RepoName(initialRunName)
			rebasedRunName = fmt.Sprintf("run-after/%s", h.RandString(10))
			rebasedRunImageName = registry.RepoName(rebasedRunName)

			assertAppImage = assert.NewImageAssertionManager(appImageName, containerManager)
		})

		it.After(func() {
			containerManager.RemoveImages(appImageName, initialRunImageName, rebasedRunImageName)
			ref, err := name.ParseReference(appImageName, name.WeakValidation)
			assert.Nil(err)
			buildCacheVolume := cache.NewVolumeCache(ref, "build", dockerCli)
			launchCacheVolume := cache.NewVolumeCache(ref, "launch", dockerCli)
			assert.Nil(buildCacheVolume.Clear(context.Background()))
			assert.Nil(launchCacheVolume.Clear(context.Background()))
		})

		when("--run-image locally", func() {
			it("uses provided run image", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				stackManager.CreateCustomRunImage(initialRunImageName, initialContents1, initialContents2)
				pack.Build(appImageName, mockAppPath, "--run-image", initialRunImageName, "--no-pull")

				assertAppImage.RunsMockAppWithOutput(initialContents1, initialContents2)

				stackManager.CreateCustomRunImage(rebasedRunImageName, rebasedContents1, rebasedContents2)

				output := pack.SuccessfulRunWithOutput(
					"rebase", appImageName,
					"--run-image", rebasedRunImageName,
					"--no-pull",
				)

				assert.NewOutputAssertionManager(output).ReportsSuccessfulRebase(appImageName)
				assertAppImage.RunsMockAppWithOutput(rebasedContents1, rebasedContents2)
			})
		})

		when("local config has a mirror", func() {
			var localRunImageMirrorName string

			it.After(func() {
				containerManager.RemoveImages(localRunImageMirrorName)
			})

			it("prefers the local mirror", func() {
				const (
					localMirrorContents1 = "local-mirror-after-1"
					localMirrorContents2 = "local-mirror-after-2"
				)

				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				stackManager.CreateCustomRunImage(initialRunImageName, initialContents1, initialContents2)
				pack.Build(appImageName, mockAppPath, "--run-image", initialRunImageName, "--no-pull")

				assertAppImage.RunsMockAppWithOutput(initialContents1, initialContents2)

				stackManager.CreateCustomRunImage(rebasedRunImageName, rebasedContents1, rebasedContents2)

				localRunImageMirrorName = registry.RepoName(fmt.Sprintf("run-after/%s", h.RandString(10)))
				stackManager.CreateCustomRunImage(localRunImageMirrorName, localMirrorContents1, localMirrorContents2)
				pack.SuccessfulRun("set-run-image-mirrors", managers.RunImage, "-m", localRunImageMirrorName)

				output := pack.SuccessfulRunWithOutput("rebase", appImageName, "--no-pull")

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsSuccessfulRebase(appImageName)
				assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirrorName)

				assertAppImage.RunsMockAppWithOutput(localMirrorContents1, localMirrorContents2)
			})
		})

		when("image metadata has a mirror", func() {
			it.After(func() {
				err := dockerCli.ImageTag(context.Background(), managers.RunImage, stackManager.RunImageMirror())
				assert.Nil(err)
			})

			it("selects the best mirror", func() {
				const (
					modifiedMirrorContent1 = "mirror-after-1"
					modifiedMirrorContent2 = "mirror-after-2"
				)

				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				stackManager.CreateCustomRunImage(initialRunImageName, initialContents1, initialContents2)
				pack.Build(appImageName, mockAppPath, "--run-image", initialRunImageName, "--no-pull")

				assertAppImage.RunsMockAppWithOutput(initialContents1, initialContents2)

				containerManager.RemoveImagesSucceeds(stackManager.RunImageMirror())
				stackManager.CreateCustomRunImage(stackManager.RunImageMirror(), modifiedMirrorContent1, modifiedMirrorContent2)

				output := pack.SuccessfulRunWithOutput("rebase", appImageName, "--no-pull")

				assertOutput := assert.NewOutputAssertionManager(output)
				assertOutput.ReportsSuccessfulRebase(appImageName)
				assertOutput.ReportsSelectingRunImageMirror(stackManager.RunImageMirror())

				assertAppImage.RunsMockAppWithOutput(modifiedMirrorContent1, modifiedMirrorContent2)
			})
		})

		when("--publish with --run-image", func() {
			it("uses provided run image", func() {
				stackManager.EnsureDefaultStackCreated()
				testBuilder := builderManager.EnsureComboBuilderExists()
				pack.SetDefaultTrustedBuilder(testBuilder.Name())

				stackManager.CreateCustomRunImage(initialRunImageName, initialContents1, initialContents2)
				pack.Build(appImageName, mockAppPath, "--run-image", initialRunImageName)
				assert.Nil(h.PushImage(dockerCli, appImageName, registry))

				assertAppImage.RunsMockAppWithOutput(initialContents1, initialContents2)

				stackManager.CreateCustomRunImageOnRemote(rebasedRunName, rebasedContents1, rebasedContents2)

				output := pack.SuccessfulRunWithOutput("rebase", appImageName, "--publish", "--run-image", rebasedRunImageName)
				assert.NewOutputAssertionManager(output).ReportsSuccessfulRebase(appImageName)

				assert.ImageExistsInRegistry(appImageName, registry)

				assertAppImage.RunsMockAppWithOutput(rebasedContents1, rebasedContents2)
			})
		})
	})
}

func dockerHostOS() string {
	daemonInfo, err := dockerCli.Info(context.TODO())
	if err != nil {
		panic(err.Error())
	}
	return daemonInfo.OSType
}

func newPackageName() string {
	return "test/package-" + h.RandString(10)
}
