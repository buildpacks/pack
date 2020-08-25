// +build acceptance

package acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/go-containerregistry/pkg/name"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/acceptance/assertions"
	"github.com/buildpacks/pack/acceptance/config"
	"github.com/buildpacks/pack/acceptance/invoke"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/cache"
	"github.com/buildpacks/pack/internal/style"
	h "github.com/buildpacks/pack/testhelpers"
)

const (
	runImage   = "pack-test/run"
	buildImage = "pack-test/build"
)

var (
	dockerCli      client.CommonAPIClient
	registryConfig *h.TestRegistryConfig
	suiteManager   *SuiteManager
)

func TestAcceptance(t *testing.T) {
	var err error

	h.RequireDocker(t)
	rand.Seed(time.Now().UTC().UnixNano())

	assert := h.NewAssertionManager(t)

	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	assert.Nil(err)

	registryConfig = h.RunRegistry(t)
	defer registryConfig.StopRegistry(t)

	inputConfigManager, err := config.NewInputConfigurationManager()
	assert.Nil(err)

	assetsConfig := config.ConvergedAssetManager(t, assert, inputConfigManager)

	suiteManager = &SuiteManager{out: t.Logf}
	suite := spec.New("acceptance suite", spec.Report(report.Terminal{}))

	if inputConfigManager.Combinations().IncludesCurrentSubjectPack() {
		suite("p_current", func(t *testing.T, when spec.G, it spec.S) {
			testWithoutSpecificBuilderRequirement(
				t,
				when,
				it,
				assetsConfig.NewPackAsset(config.Current),
			)
		}, spec.Report(report.Terminal{}))
	}

	for _, combo := range inputConfigManager.Combinations() {
		t.Logf(`setting up run combination %s: %s`,
			style.Symbol(combo.String()),
			combo.Describe(assetsConfig),
		)

		suite(combo.String(), func(t *testing.T, when spec.G, it spec.S) {
			testAcceptance(
				t,
				when,
				it,
				assetsConfig.NewPackAsset(combo.Pack),
				assetsConfig.NewPackAsset(combo.PackCreateBuilder),
				assetsConfig.NewLifecycleAsset(combo.Lifecycle),
			)
		}, spec.Report(report.Terminal{}))
	}

	suite.Run(t)

	assert.Nil(suiteManager.CleanUp())
}

// These tests either (a) do not require a builder or (b) do not require a specific builder to be provided
// in order to test compatibility.
// They should only be run against the "current" (i.e., main) version of pack.
func testWithoutSpecificBuilderRequirement(
	t *testing.T,
	when spec.G,
	it spec.S,
	packConfig config.PackAsset,
) {
	var (
		bpDir  = buildpacksDir(builder.DefaultBuildpackAPIVersion)
		pack   *invoke.PackInvoker
		assert = h.NewAssertionManager(t)
	)

	it.Before(func() {
		pack = invoke.NewPackInvoker(t, assert, packConfig, registryConfig.DockerConfigDir)
	})

	it.After(func() {
		pack.Cleanup()
	})

	when("invalid subcommand", func() {
		it("prints usage", func() {
			output, err := pack.Run("some-bad-command")
			assert.NotNil(err)

			assertOutput := assertions.NewOutputAssertionManager(t, output)
			assertOutput.ReportsCommandUnknown("some-bad-command")
			assertOutput.IncludesUsagePrompt()
		})
	})

	when("suggest-builders", func() {
		it("displays suggested builders", func() {
			output := pack.RunSuccessfully("suggest-builders")

			assertOutput := assertions.NewOutputAssertionManager(t, output)
			assertOutput.IncludesSuggestedBuildersHeading()
			assertOutput.IncludesPrefixedGoogleBuilder()
			assertOutput.IncludesPrefixedHerokuBuilder()
			assertOutput.IncludesPrefixedPaketoBuilders()
		})
	})

	when("suggest-stacks", func() {
		it("displays suggested stacks", func() {
			output, err := pack.Run("suggest-stacks")
			assert.NilWithMessage(err, fmt.Sprintf("suggest-stacks command failed with output %s", output))

			assertions.NewOutputAssertionManager(t, output).IncludesSuggestedStacksHeading()
		})
	})

	when("set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			builderName := "paketobuildpacks/builder:base"
			output := pack.RunSuccessfully("set-default-builder", builderName)

			assertions.NewOutputAssertionManager(t, output).ReportsSettingDefaultBuilder(builderName)
		})
	})

	when("trust-builder", func() {
		it("sets the builder as trusted in ~/.pack/config.toml", func() {
			h.SkipUnless(t, pack.Supports("trust-builder"), "pack does not support 'trust-builder'")
			builderName := "some-builder" + h.RandString(10)

			pack.JustRunSuccessfully("trust-builder", builderName)

			assert.Contains(pack.ConfigFileContents(), builderName)
		})
	})

	when("untrust-builder", func() {
		it("removes the previously trusted builder from ~/${PACK_HOME}/config.toml", func() {
			h.SkipUnless(t, pack.Supports("untrust-builder"), "pack does not support 'untrust-builder'")
			builderName := "some-builder" + h.RandString(10)

			pack.JustRunSuccessfully("trust-builder", builderName)

			assert.Contains(pack.ConfigFileContents(), builderName)

			pack.JustRunSuccessfully("untrust-builder", builderName)

			assert.NotContain(pack.ConfigFileContents(), builderName)
		})
	})

	when("list-trusted-builders", func() {
		it.Before(func() {
			h.SkipUnless(t,
				pack.Supports("list-trusted-builders"),
				"pack does not support 'list-trusted-builders",
			)
		})

		it("shows default builders from pack suggest-builders", func() {
			output := pack.RunSuccessfully("list-trusted-builders")

			assertOutput := assertions.NewOutputAssertionManager(t, output)
			assertOutput.IncludesTrustedBuildersHeading()
			assertOutput.IncludesHerokuBuilder()
			assertOutput.IncludesGoogleBuilder()
			assertOutput.IncludesPaketoBuilders()
		})

		it("shows a builder trusted by pack trust-builder", func() {
			builderName := "some-builder" + h.RandString(10)

			pack.JustRunSuccessfully("trust-builder", builderName)

			output := pack.RunSuccessfully("list-trusted-builders")
			assert.Contains(output, builderName)
		})
	})

	when("package-buildpack", func() {
		var (
			tmpDir                  string
			simplePackageConfigPath string
		)

		it.Before(func() {
			h.SkipUnless(t,
				pack.Supports("package-buildpack"),
				"pack does not support 'package-buildpack'",
			)

			h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

			var err error
			tmpDir, err = ioutil.TempDir("", "package-buildpack-tests")
			assert.Nil(err)

			simplePackageConfigPath = filepath.Join(tmpDir, "package.toml")
			h.CopyFile(t, pack.FixtureManager().FixtureLocation("package.toml"), simplePackageConfigPath)

			err = os.Rename(
				h.CreateTGZ(t, filepath.Join(bpDir, "simple-layers-parent-buildpack"), "./", 0755),
				filepath.Join(tmpDir, "simple-layers-parent-buildpack.tgz"),
			)
			assert.Nil(err)

			err = os.Rename(
				h.CreateTGZ(t, filepath.Join(bpDir, "simple-layers-buildpack"), "./", 0755),
				filepath.Join(tmpDir, "simple-layers-buildpack.tgz"),
			)
			assert.Nil(err)
		})

		it.After(func() {
			assert.Nil(os.RemoveAll(tmpDir))
		})

		packageBuildpackLocally := func(absConfigPath string) string {
			t.Helper()
			packageName := "test/package-" + h.RandString(10)
			output, err := pack.Run("package-buildpack", packageName, "-c", absConfigPath)
			assert.Nil(err)

			assertions.NewOutputAssertionManager(t, output).ReportsPackageCreation(packageName)
			return packageName
		}

		packageBuildpackRemotely := func(absConfigPath string) string {
			t.Helper()
			packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
			output, err := pack.Run("package-buildpack", packageName, "-c", absConfigPath, "--publish")
			assert.Nil(err)
			assertions.NewOutputAssertionManager(t, output).ReportsPackagePublished(packageName)
			return packageName
		}

		assertImageExistsLocally := func(name string) {
			t.Helper()
			_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), name)
			assert.Nil(err)

		}

		generateAggregatePackageToml := func(buildpackURI, nestedPackageName string) string {
			t.Helper()
			packageTomlFile, err := ioutil.TempFile(tmpDir, "package_aggregate-*.toml")
			assert.Nil(err)

			pack.FixtureManager().TemplateFixtureToFile(
				"package_aggregate.toml",
				packageTomlFile,
				map[string]interface{}{
					"BuildpackURI": buildpackURI,
					"PackageName":  nestedPackageName,
				},
			)

			assert.Nil(packageTomlFile.Close())

			return packageTomlFile.Name()
		}

		when("no --format is provided", func() {
			it("creates the package as image", func() {
				packageName := "test/package-" + h.RandString(10)
				output := pack.RunSuccessfully("package-buildpack", packageName, "-c", simplePackageConfigPath)
				assertions.NewOutputAssertionManager(t, output).ReportsPackageCreation(packageName)
				defer h.DockerRmi(dockerCli, packageName)

				assertImageExistsLocally(packageName)
			})
		})

		when("--format image", func() {
			it("creates the package", func() {
				t.Log("package w/ only buildpacks")
				nestedPackageName := packageBuildpackLocally(simplePackageConfigPath)
				defer h.DockerRmi(dockerCli, nestedPackageName)
				assertImageExistsLocally(nestedPackageName)

				t.Log("package w/ buildpacks and packages")
				aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackageName)
				packageName := packageBuildpackLocally(aggregatePackageToml)
				defer h.DockerRmi(dockerCli, packageName)
				assertImageExistsLocally(packageName)
			})

			when("--publish", func() {
				it("publishes image to registry", func() {
					nestedPackageName := packageBuildpackRemotely(simplePackageConfigPath)
					defer h.DockerRmi(dockerCli, nestedPackageName)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackageName)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					output := pack.RunSuccessfully(
						"package-buildpack", packageName,
						"-c", aggregatePackageToml,
						"--publish",
					)
					assertions.NewOutputAssertionManager(t, output).ReportsPackagePublished(packageName)

					_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					h.AssertError(t, err, "No such image")

					assert.Nil(h.PullImageWithAuth(dockerCli, packageName, registryConfig.RegistryAuth()))

					_, _, err = dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					assert.Nil(err)
				})
			})

			when("--no-pull", func() {
				it("should use local image", func() {
					nestedPackage := packageBuildpackLocally(simplePackageConfigPath)
					defer h.DockerRmi(dockerCli, nestedPackage)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackage)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					// TODO: Replace --no-pull with pull-policy never. See https://github.com/buildpacks/pack/issues/775
					pack.JustRunSuccessfully(
						"package-buildpack", packageName,
						"-c", aggregatePackageToml,
						"--no-pull",
					)

					_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					assert.Nil(err)

				})

				it("should not pull image from registry", func() {
					nestedPackage := packageBuildpackRemotely(simplePackageConfigPath)
					defer h.DockerRmi(dockerCli, nestedPackage)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackage)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					// TODO: Replace --no-pull with pull-policy never. See https://github.com/buildpacks/pack/issues/775
					output, err := pack.Run(
						"package-buildpack", packageName,
						"-c", aggregatePackageToml,
						"--no-pull",
					)
					assert.NotNil(err)
					assertions.NewOutputAssertionManager(t, output).ReportsImageNotExistingOnDaemon(nestedPackage)
				})
			})
		})

		when("--format file", func() {
			it.Before(func() {
				h.SkipIf(t, !pack.Supports("package-buildpack --format"), "format not supported")
			})

			it("creates the package", func() {
				destinationFile := filepath.Join(tmpDir, "package.cnb")
				output, err := pack.Run(
					"package-buildpack", destinationFile,
					"--format", "file",
					"-c", simplePackageConfigPath,
				)
				assert.Nil(err)
				assertions.NewOutputAssertionManager(t, output).ReportsPackageCreation(destinationFile)
				h.AssertTarball(t, destinationFile)
			})
		})

		when("package.toml is invalid", func() {
			it("displays an error", func() {
				output, err := pack.Run(
					"package-buildpack", "some-package",
					"-c", pack.FixtureManager().FixtureLocation("invalid_package.toml"),
				)
				assert.NotNil(err)
				assert.Contains(output, "reading config")
			})
		})
	})

	when("report", func() {
		it.Before(func() {
			h.SkipIf(t, !pack.Supports("report"), "pack does not support 'report' command")
		})

		when("default builder is set", func() {
			it("redacts default builder", func() {
				pack.RunSuccessfully("set-default-builder", "paketobuildpacks/builder:base")

				output := pack.RunSuccessfully("report")

				version := pack.Version()

				expectedOutput := pack.FixtureManager().TemplateFixture(
					"report_output.txt",
					map[string]interface{}{
						"DefaultBuilder": "[REDACTED]",
						"Version":        version,
						"OS":             runtime.GOOS,
						"Arch":           runtime.GOARCH,
					},
				)
				assert.Equal(output, expectedOutput)
			})

			it("explicit mode doesn't redact", func() {
				pack.RunSuccessfully("set-default-builder", "paketobuildpacks/builder:base")

				output := pack.RunSuccessfully("report", "--explicit")

				version := pack.Version()

				expectedOutput := pack.FixtureManager().TemplateFixture(
					"report_output.txt",
					map[string]interface{}{
						"DefaultBuilder": "paketobuildpacks/builder:base",
						"Version":        version,
						"OS":             runtime.GOOS,
						"Arch":           runtime.GOARCH,
					},
				)
				assert.Equal(output, expectedOutput)
			})
		})
	})

	when("build with default builders not set", func() {
		it("informs the user", func() {
			output, err := pack.Run(
				"build", "some/image",
				"-p", filepath.Join("testdata", "mock_app"),
			)

			assert.NotNil(err)
			assertOutput := assertions.NewOutputAssertionManager(t, output)
			assertOutput.IncludesMessageToSetDefaultBuilder()
			assertOutput.IncludesPrefixedGoogleBuilder()
			assertOutput.IncludesPrefixedHerokuBuilder()
			assertOutput.IncludesPrefixedPaketoBuilders()
		})
	})
}

func testAcceptance(
	t *testing.T,
	when spec.G,
	it spec.S,
	subjectPackConfig, createBuilderPackConfig config.PackAsset,
	lifecycle config.LifecycleAsset,
) {
	var (
		pack, createBuilderPack *invoke.PackInvoker
		bpDir                   = buildpacksDir(lifecycle.EarliestBuildpackAPIVersion())
		assert                  = h.NewAssertionManager(t)
	)

	it.Before(func() {
		pack = invoke.NewPackInvoker(t, assert, subjectPackConfig, registryConfig.DockerConfigDir)
		pack.EnableExperimental()
		createBuilderPack = invoke.NewPackInvoker(t, assert, createBuilderPackConfig, registryConfig.DockerConfigDir)
		createBuilderPack.EnableExperimental()
	})

	it.After(func() {
		pack.Cleanup()
		createBuilderPack.Cleanup()
	})

	when("stack is created", func() {
		var (
			runImageMirror string
		)

		it.Before(func() {
			value, err := suiteManager.RunTaskOnceString("create-stack",
				func() (string, error) {
					runImageMirror := registryConfig.RepoName(runImage)
					err := createStack(t, dockerCli, runImageMirror)
					if err != nil {
						return "", err
					}

					return runImageMirror, nil
				})
			assert.Nil(err)

			suiteManager.RegisterCleanUp("remove-stack-images", func() error {
				return h.DockerRmi(dockerCli, runImage, buildImage, value)
			})

			runImageMirror = value
		})

		when("builder is created", func() {
			var (
				builderName string
				tmpDir      string
			)

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "package-buildpack-tests")
				assert.Nil(err)

				key := taskKey(
					"create-builder",
					append(
						[]string{runImageMirror, createBuilderPackConfig.Path(), lifecycle.Identifier()},
						createBuilderPackConfig.FixturePaths()...,
					)...,
				)
				value, err := suiteManager.RunTaskOnceString(key, func() (string, error) {
					return createBuilder(t, assert, createBuilderPack, lifecycle, runImageMirror)
				})
				assert.Nil(err)
				suiteManager.RegisterCleanUp("clean-"+key, func() error {
					return h.DockerRmi(dockerCli, value)
				})

				builderName = value
			})

			it.After(func() {
				assert.Nil(os.RemoveAll(tmpDir))
			})

			when("builder.toml is invalid", func() {
				it("displays an error", func() {
					h.SkipUnless(
						t,
						createBuilderPack.SupportsFeature(invoke.BuilderTomlValidation),
						"builder.toml validation not supported",
					)

					builderConfigPath := createBuilderPack.FixtureManager().FixtureLocation("invalid_builder.toml")

					output, err := pack.Run(
						"create-builder", "some-builder:build",
						"--config", builderConfigPath,
					)
					assert.NotNil(err)
					assert.Contains(output, "invalid builder toml")
				})
			})

			when("build", func() {
				var repo, repoName string

				it.Before(func() {
					repo = "some-org/" + h.RandString(10)
					repoName = registryConfig.RepoName(repo)
				})

				it.After(func() {
					h.DockerRmi(dockerCli, repoName)
					ref, err := name.ParseReference(repoName, name.WeakValidation)
					assert.Nil(err)
					cacheImage := cache.NewImageCache(ref, dockerCli)
					buildCacheVolume := cache.NewVolumeCache(ref, "build", dockerCli)
					launchCacheVolume := cache.NewVolumeCache(ref, "launch", dockerCli)
					cacheImage.Clear(context.TODO())
					buildCacheVolume.Clear(context.TODO())
					launchCacheVolume.Clear(context.TODO())
				})

				when("builder is untrusted", func() {
					var untrustedBuilderName string

					it.Before(func() {
						h.SkipIf(t, dockerHostOS() == "windows", "untrusted builders are not yet supported for windows builds")

						var err error
						untrustedBuilderName, err = createBuilder(
							t,
							assert,
							createBuilderPack,
							lifecycle,
							runImageMirror,
						)
						assert.Nil(err)
					})

					it.After(func() {
						if dockerHostOS() == "windows" {
							return
						}
						h.DockerRmi(dockerCli, untrustedBuilderName)
					})

					it("uses the 5 phases", func() {
						output := pack.RunSuccessfully(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"-B", untrustedBuilderName,
						)

						assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)

						assertOutput := assertions.NewLifecycleOutputAssertionManager(t, output)

						if pack.SupportsFeature(invoke.CreatorInPack) {
							assertOutput.IncludesLifecycleImageTag()
						}
						assertOutput.IncludesSeparatePhases()
					})
				})

				when("default builder is set", func() {
					var usingCreator bool

					it.Before(func() {
						pack.JustRunSuccessfully("set-default-builder", builderName)

						var trustBuilder bool
						if pack.Supports("trust-builder") {
							pack.JustRunSuccessfully("trust-builder", builderName)
							trustBuilder = true
						}

						// Technically the creator is supported as of platform API version 0.3 (lifecycle version 0.7.0+) but earlier versions
						// have bugs that make using the creator problematic.
						creatorSupported := lifecycle.SupportsFeature(config.CreatorInLifecycle) &&
							pack.SupportsFeature(invoke.CreatorInPack)

						usingCreator = creatorSupported && trustBuilder
					})

					it("creates a runnable, rebuildable image on daemon from app dir", func() {
						appPath := filepath.Join("testdata", "mock_app")

						output := pack.RunSuccessfully(
							"build", repoName,
							"-p", appPath,
						)

						imgId, err := imgIDForRepoName(repoName)
						if err != nil {
							t.Fatal(err)
						}
						defer h.DockerRmi(dockerCli, imgId)

						assertOutput := assertions.NewOutputAssertionManager(t, output)

						assertOutput.ReportsSuccessfulImageBuild(repoName)
						assertOutput.ReportsUsingBuildCacheVolume()
						assertOutput.ReportsSelectingRunImageMirror(runImageMirror)

						t.Log("app is runnable")
						assertMockAppRunsWithOutput(t, assert, repoName, "Launch Dep Contents", "Cached Dep Contents")

						t.Log("it uses the run image as a base image")
						assertHasBase(t, assert, repoName, runImage)

						t.Log("sets the run image metadata")
						appMetadataLabel := imageLabel(t,
							assert,
							dockerCli,
							repoName,
							"io.buildpacks.lifecycle.metadata",
						)
						assert.Contains(appMetadataLabel, fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImage, runImageMirror))

						t.Log("registry is empty")
						contents, err := registryConfig.RegistryCatalog()
						assert.Nil(err)
						if strings.Contains(contents, repo) {
							t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
						}

						t.Log("add a local mirror")
						localRunImageMirror := registryConfig.RepoName("pack-test/run-mirror")
						assert.Nil(dockerCli.ImageTag(context.TODO(), runImage, localRunImageMirror))
						defer h.DockerRmi(dockerCli, localRunImageMirror)
						pack.JustRunSuccessfully("set-run-image-mirrors", runImage, "-m", localRunImageMirror)

						t.Log("rebuild")
						output = pack.RunSuccessfully(
							"build", repoName,
							"-p", appPath,
						)
						assertOutput.ReportsSuccessfulImageBuild(repoName)

						imgId, err = imgIDForRepoName(repoName)
						if err != nil {
							t.Fatal(err)
						}
						defer h.DockerRmi(dockerCli, imgId)

						assertOutput = assertions.NewOutputAssertionManager(t, output)
						assertOutput.ReportsSuccessfulImageBuild(repoName)
						assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirror)
						cachedLaunchLayer := "simple/layers:cached-launch-layer"

						assertLifecycleOutput := assertions.NewLifecycleOutputAssertionManager(t, output)
						assertLifecycleOutput.ReportsRestoresCachedLayer(cachedLaunchLayer)
						assertLifecycleOutput.ReportsExporterReusingUnchangedLayer(cachedLaunchLayer)
						assertLifecycleOutput.ReportsCacheReuse(cachedLaunchLayer)

						t.Log("app is runnable")
						assertMockAppRunsWithOutput(t, assert, repoName, "Launch Dep Contents", "Cached Dep Contents")

						t.Log("rebuild with --clear-cache")
						output = pack.RunSuccessfully("build", repoName, "-p", appPath, "--clear-cache")

						assertOutput = assertions.NewOutputAssertionManager(t, output)
						assertOutput.ReportsSuccessfulImageBuild(repoName)
						if !usingCreator {
							assertOutput.ReportsSkippingRestore()
						}

						assertLifecycleOutput = assertions.NewLifecycleOutputAssertionManager(t, output)
						assertLifecycleOutput.ReportsSkippingBuildpackLayerAnalysis()
						assertLifecycleOutput.ReportsExporterReusingUnchangedLayer(cachedLaunchLayer)
						assertLifecycleOutput.ReportsCacheCreation(cachedLaunchLayer)

						t.Log("cacher adds layers")
						h.AssertContainsMatch(t, output, `(?i)Adding cache layer 'simple/layers:cached-launch-layer'`)

						if pack.Supports("inspect-image") {
							t.Log("inspect-image")
							output = pack.RunSuccessfully("inspect-image", repoName)

							var (
								webCommand   string
								helloCommand string
								helloArgs    string
							)
							if dockerHostOS() == "windows" {
								webCommand = ".\\run"
								helloCommand = "cmd"
								helloArgs = " /c echo hello world"
							} else {
								webCommand = "./run"
								helloCommand = "echo"
								helloArgs = "hello world"
							}

							expectedOutput := pack.FixtureManager().TemplateFixture(
								"inspect_image_local_output.txt",
								map[string]interface{}{
									"image_name":             repoName,
									"base_image_id":          h.ImageID(t, runImageMirror),
									"base_image_top_layer":   h.TopLayerDiffID(t, runImageMirror),
									"run_image_local_mirror": localRunImageMirror,
									"run_image_mirror":       runImageMirror,
									"web_command":            webCommand,
									"hello_command":          helloCommand,
									"hello_args":             helloArgs,
								},
							)

							assert.Equal(output, expectedOutput)
						}
					})

					when("--no-color", func() {
						it.Before(func() {
							h.SkipUnless(t,
								pack.SupportsFeature(invoke.NoColorInBuildpacks),
								"pack had a no-color bug for color strings in buildpacks until 0.12.0",
							)
						})

						it("doesn't have color", func() {
							appPath := filepath.Join("testdata", "mock_app")

							output := pack.RunSuccessfully("build", repoName, "-p", appPath)
							imgId, err := imgIDForRepoName(repoName)
							if err != nil {
								t.Fatal(err)
							}
							defer h.DockerRmi(dockerCli, imgId)

							assertOutput := assertions.NewOutputAssertionManager(t, output)

							assertOutput.ReportsSuccessfulImageBuild(repoName)
							assertOutput.WithoutColors()
						})
					})

					it("supports building app from a zip file", func() {
						appPath := filepath.Join("testdata", "mock_app.zip")
						output := pack.RunSuccessfully("build", repoName, "-p", appPath)
						assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)

						imgId, err := imgIDForRepoName(repoName)
						if err != nil {
							t.Fatal(err)
						}
						defer h.DockerRmi(dockerCli, imgId)
					})

					when("--network", func() {
						var buildpackTgz string

						it.Before(func() {
							h.SkipUnless(t,
								pack.Supports("build --network"),
								"--network flag not supported for build",
							)

							buildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "internet-capable-buildpack"), "./", 0755)
						})

						it.After(func() {
							assert.Nil(os.Remove(buildpackTgz))
							assert.Nil(h.DockerRmi(dockerCli, repoName))
						})

						when("the network mode is not provided", func() {
							it("reports that build and detect are online", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
								)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsConnectedToInternet()
							})
						})

						when("the network mode is set to default", func() {
							it("reports that build and detect are online", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
								)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsConnectedToInternet()
							})
						})

						when("the network mode is set to none", func() {
							it("reports that build and detect are offline", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
									"--network", "none",
								)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsDisconnectedFromInternet()
							})
						})
					})

					when("--volume", func() {
						var (
							readBuildpackTgz,
							readWriteBuildpackTgz,
							tmpVolumeSrc string
							volumeRoot = "/"
							slash      = "/"
						)

						it.Before(func() {
							h.SkipIf(t, os.Getenv("DOCKER_HOST") != "", "cannot mount volume when DOCKER_HOST is set")

							h.SkipUnless(t,
								pack.SupportsFeature(invoke.ReadWriteVolumeMounts),
								"pack version does not support read/write volume mounts",
							)

							if dockerHostOS() == "windows" {
								volumeRoot = `c:\`
								slash = `\`
							}
							readBuildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "read-volume-buildpack"), "./", 0755)
							readWriteBuildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "read-write-volume-buildpack"), "./", 0755)

							var err error
							tmpVolumeSrc, err = ioutil.TempDir("", "volume-mount-source")
							assert.Nil(err)
							assert.Nil(os.Chmod(tmpVolumeSrc, 0777)) // Override umask

							// Some OSes (like macOS) use symlinks for the standard temp dir.
							// Resolve it so it can be properly mounted by the Docker daemon.
							tmpVolumeSrc, err = filepath.EvalSymlinks(tmpVolumeSrc)
							assert.Nil(err)

							err = ioutil.WriteFile(filepath.Join(tmpVolumeSrc, "some-file"), []byte("some-content\n"), 0777)
							assert.Nil(err)
						})

						it.After(func() {
							_ = os.Remove(readBuildpackTgz)
							_ = os.Remove(readWriteBuildpackTgz)
							_ = h.DockerRmi(dockerCli, repoName)
							_ = os.RemoveAll(tmpVolumeSrc)
						})

						when("volume is read-only", func() {
							it("mounts the provided volume in the detect and build phases", func() {
								volumeDest := volumeRoot + "platform" + slash + "volume-mount-target"
								testFilePath := volumeDest + slash + "some-file"
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--volume", fmt.Sprintf("%s:%s", tmpVolumeSrc, volumeDest),
									"--buildpack", readBuildpackTgz,
									"--env", "TEST_FILE_PATH="+testFilePath,
								)

								bpOutputAsserts := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								bpOutputAsserts.ReportsReadingFileContents("Detect", testFilePath, "some-content")
								bpOutputAsserts.ReportsReadingFileContents("Build", testFilePath, "some-content")
							})

							it("should fail to write", func() {
								volumeDest := volumeRoot + "platform" + slash + "volume-mount-target"
								testDetectFilePath := volumeDest + slash + "detect-file"
								testBuildFilePath := volumeDest + slash + "build-file"
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--volume", fmt.Sprintf("%s:%s", tmpVolumeSrc, volumeDest),
									"--buildpack", readWriteBuildpackTgz,
									"--env", "DETECT_TEST_FILE_PATH="+testDetectFilePath,
									"--env", "BUILD_TEST_FILE_PATH="+testBuildFilePath,
								)

								bpOutputAsserts := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								bpOutputAsserts.ReportsFailingToWriteFileContents("Detect", testDetectFilePath)
								bpOutputAsserts.ReportsFailingToWriteFileContents("Build", testBuildFilePath)
							})
						})

						when("volume is read-write", func() {
							it("can be written to", func() {
								volumeDest := volumeRoot + "volume-mount-target"
								testDetectFilePath := volumeDest + slash + "detect-file"
								testBuildFilePath := volumeDest + slash + "build-file"
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--volume", fmt.Sprintf("%s:%s:rw", tmpVolumeSrc, volumeDest),
									"--buildpack", readWriteBuildpackTgz,
									"--env", "DETECT_TEST_FILE_PATH="+testDetectFilePath,
									"--env", "BUILD_TEST_FILE_PATH="+testBuildFilePath,
								)

								bpOutputAsserts := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								bpOutputAsserts.ReportsWritingFileContents("Detect", testDetectFilePath)
								bpOutputAsserts.ReportsReadingFileContents("Detect", testDetectFilePath, "some-content")
								bpOutputAsserts.ReportsWritingFileContents("Build", testBuildFilePath)
								bpOutputAsserts.ReportsReadingFileContents("Build", testBuildFilePath, "some-content")
							})
						})
					})

					when("--default-process", func() {
						it("sets the default process from those in the process list", func() {
							pack.RunSuccessfully(
								"build", repoName,
								"--default-process", "hello",
								"-p", filepath.Join("testdata", "mock_app"),
							)

							assertMockAppLogs(t, assert, repoName, "hello world")
						})
					})

					when("--buildpack", func() {
						when("the argument is an ID", func() {
							it("adds the buildpacks to the builder if necessary and runs them", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", "simple/layers", // can omit version if only one
									"--buildpack", "noop.buildpack@noop.buildpack.version",
								)

								assertOutput := assertions.NewOutputAssertionManager(t, output)

								assertTestAppOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertTestAppOutput.ReportsBuildStep("Simple Layers Buildpack")
								assertTestAppOutput.ReportsBuildStep("NOOP Buildpack")
								assertOutput.ReportsSuccessfulImageBuild(repoName)

								t.Log("app is runnable")
								assertMockAppRunsWithOutput(t,
									assert,
									repoName,
									"Launch Dep Contents",
									"Cached Dep Contents",
								)
							})
						})

						when("the argument is an archive", func() {
							var localBuildpackTgz string

							it.Before(func() {
								localBuildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "not-in-builder-buildpack"), "./", 0755)
							})

							it.After(func() {
								assert.Nil(os.Remove(localBuildpackTgz))
							})

							it("adds the buildpack to the builder and runs it", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", localBuildpackTgz,
								)

								assertOutput := assertions.NewOutputAssertionManager(t, output)
								assertOutput.ReportsAddingBuildpack("local/bp", "local-bp-version")
								assertOutput.ReportsSuccessfulImageBuild(repoName)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsBuildStep("Local Buildpack")
							})
						})

						when("the argument is directory", func() {
							it("adds the buildpacks to the builder and runs it", func() {
								h.SkipIf(t, runtime.GOOS == "windows", "buildpack directories not supported on windows")

								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", filepath.Join(bpDir, "not-in-builder-buildpack"),
								)

								assertOutput := assertions.NewOutputAssertionManager(t, output)
								assertOutput.ReportsAddingBuildpack("local/bp", "local-bp-version")
								assertOutput.ReportsSuccessfulImageBuild(repoName)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsBuildStep("Local Buildpack")
							})
						})

						when("the argument is a buildpackage image", func() {
							var packageImageName string

							it.Before(func() {
								h.SkipIf(t,
									!pack.Supports("package-buildpack"),
									"--buildpack does not accept buildpackage unless package-buildpack is supported",
								)
								h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

								packageImageName = packageBuildpackAsImage(t,
									assert,
									pack,
									pack.FixtureManager().FixtureLocation("package_for_build_cmd.toml"),
									lifecycle,
									[]string{
										"simple-layers-parent-buildpack",
										"simple-layers-buildpack",
									},
								)
							})

							it("adds the buildpacks to the builder and runs them", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", packageImageName,
								)

								assertOutput := assertions.NewOutputAssertionManager(t, output)
								assertOutput.ReportsAddingBuildpack(
									"simple/layers/parent",
									"simple-layers-parent-version",
								)
								assertOutput.ReportsAddingBuildpack("simple/layers", "simple-layers-version")
								assertOutput.ReportsSuccessfulImageBuild(repoName)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsBuildStep("Simple Layers Buildpack")
							})
						})

						when("the argument is a buildpackage file", func() {
							var (
								packageFile string
								tmpDir      string
							)

							it.Before(func() {
								h.SkipIf(t,
									!pack.Supports("package-buildpack --format"),
									"--buildpack does not accept buildpackage file unless package-buildpack with --format is supported",
								)
								h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

								var err error
								tmpDir, err = ioutil.TempDir("", "package-file")
								assert.Nil(err)

								packageFile = packageBuildpackAsFile(t,
									assert,
									pack,
									pack.FixtureManager().FixtureLocation("package_for_build_cmd.toml"),
									tmpDir,
									lifecycle,
									[]string{
										"simple-layers-parent-buildpack",
										"simple-layers-buildpack",
									},
								)
							})

							it.After(func() {
								assert.Nil(os.RemoveAll(tmpDir))
							})

							it("adds the buildpacks to the builder and runs them", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", packageFile,
								)

								assertOutput := assertions.NewOutputAssertionManager(t, output)
								assertOutput.ReportsAddingBuildpack(
									"simple/layers/parent",
									"simple-layers-parent-version",
								)
								assertOutput.ReportsAddingBuildpack("simple/layers", "simple-layers-version")
								assertOutput.ReportsSuccessfulImageBuild(repoName)

								assertBuildpackOutput := assertions.NewTestBuildpackOutputAssertionManager(t, output)
								assertBuildpackOutput.ReportsBuildStep("Simple Layers Buildpack")
							})
						})

						when("the buildpack stack doesn't match the builder", func() {
							var otherStackBuilderTgz string

							it.Before(func() {
								otherStackBuilderTgz = h.CreateTGZ(t, filepath.Join(bpDir, "other-stack-buildpack"), "./", 0755)
							})

							it.After(func() {
								assert.Nil(os.Remove(otherStackBuilderTgz))
							})

							it("errors", func() {
								output, err := pack.Run(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", otherStackBuilderTgz,
								)

								assert.NotNil(err)
								assert.Contains(output, "other/stack/bp")
								assert.Contains(output, "other-stack-version")
								assert.Contains(output, "does not support stack 'pack.test.stack'")
							})
						})
					})

					when("--env-file", func() {
						var envPath string

						it.Before(func() {
							envfile, err := ioutil.TempFile("", "envfile")
							assert.Nil(err)
							defer envfile.Close()

							err = os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment")
							assert.Nil(err)
							envfile.WriteString(`
            DETECT_ENV_BUILDPACK=true
			ENV1_CONTENTS=Env1 Layer Contents From File
			ENV2_CONTENTS
			`)
							envPath = envfile.Name()
						})

						it.After(func() {
							assert.Nil(os.Unsetenv("ENV2_CONTENTS"))
							assert.Nil(os.RemoveAll(envPath))
						})

						it("provides the env vars to the build and detect steps", func() {
							output := pack.RunSuccessfully(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--env-file", envPath,
							)

							assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"Env2 Layer Contents From Environment",
								"Env1 Layer Contents From File",
							)
						})
					})

					when("--env", func() {
						it.Before(func() {
							assert.Nil(os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment"))
						})

						it.After(func() {
							assert.Nil(os.Unsetenv("ENV2_CONTENTS"))
						})

						it("provides the env vars to the build and detect steps", func() {
							output := pack.RunSuccessfully(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--env", "DETECT_ENV_BUILDPACK=true",
								"--env", `ENV1_CONTENTS="Env1 Layer Contents From Command Line"`,
								"--env", "ENV2_CONTENTS",
							)

							assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"Env2 Layer Contents From Environment",
								"Env1 Layer Contents From Command Line",
							)
						})
					})

					when("--run-image", func() {
						var runImageName string

						when("the run-image has the correct stack ID", func() {
							it.Before(func() {
								user := func() string {
									if dockerHostOS() == "windows" {
										return "ContainerAdministrator"
									}

									return "root"
								}

								runImageName = h.CreateImageOnRemote(t, dockerCli, registryConfig, "custom-run-image"+h.RandString(10), fmt.Sprintf(`
													FROM %s
													USER %s
													RUN echo "custom-run" > /custom-run.txt
													USER pack
												`, runImage, user()))
							})

							it.After(func() {
								h.DockerRmi(dockerCli, runImageName)
							})

							it("uses the run image as the base image", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--run-image", runImageName,
								)
								assertOutput := assertions.NewOutputAssertionManager(t, output)
								assertOutput.ReportsSuccessfulImageBuild(repoName)
								assertOutput.ReportsPullingImage(runImageName)

								t.Log("app is runnable")
								assertMockAppRunsWithOutput(t,
									assert,
									repoName,
									"Launch Dep Contents",
									"Cached Dep Contents",
								)

								t.Log("uses the run image as the base image")
								assertHasBase(t, assert, repoName, runImageName)
							})
						})

						when("the run image has the wrong stack ID", func() {
							it.Before(func() {
								runImageName = h.CreateImageOnRemote(t, dockerCli, registryConfig, "custom-run-image"+h.RandString(10), fmt.Sprintf(`
													FROM %s
													LABEL io.buildpacks.stack.id=other.stack.id
													USER pack
												`, runImage))

							})

							it.After(func() {
								h.DockerRmi(dockerCli, runImageName)
							})

							it("fails with a message", func() {
								output, err := pack.Run(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--run-image", runImageName,
								)
								assert.NotNil(err)

								assertOutput := assertions.NewOutputAssertionManager(t, output)
								assertOutput.ReportsRunImageStackNotMatchingBuilder(
									"other.stack.id",
									"pack.test.stack",
								)
							})
						})
					})

					when("--publish", func() {
						it("creates image on the registry", func() {
							buildArgs := []string{
								repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--publish",
							}
							if dockerHostOS() != "windows" {
								buildArgs = append(buildArgs, "--network", "host")
							}

							output := pack.RunSuccessfully("build", buildArgs...)
							assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulImageBuild(repoName)

							t.Log("checking that registry has contents")
							contents, err := registryConfig.RegistryCatalog()
							assert.Nil(err)
							if !strings.Contains(contents, repo) {
								t.Fatalf("Expected to see image %s in %s", repo, contents)
							}

							assert.Nil(h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
							defer h.DockerRmi(dockerCli, repoName)

							t.Log("app is runnable")
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"Launch Dep Contents",
								"Cached Dep Contents",
							)

							if pack.Supports("inspect-image") {
								t.Log("inspect-image")
								output = pack.RunSuccessfully("inspect-image", repoName)

								var (
									webCommand   string
									helloCommand string
									helloArgs    string
								)
								if dockerHostOS() == "windows" {
									webCommand = ".\\run"
									helloCommand = "cmd"
									helloArgs = " /c echo hello world"
								} else {
									webCommand = "./run"
									helloCommand = "echo"
									helloArgs = "hello world"
								}

								expectedOutput := pack.FixtureManager().TemplateFixture(
									"inspect_image_published_output.txt",
									map[string]interface{}{
										"image_name":           repoName,
										"base_image_ref":       strings.Join([]string{runImageMirror, h.Digest(t, runImageMirror)}, "@"),
										"base_image_top_layer": h.TopLayerDiffID(t, runImageMirror),
										"run_image_mirror":     runImageMirror,
										"web_command":          webCommand,
										"hello_command":        helloCommand,
										"hello_args":           helloArgs,
									},
								)

								assert.Equal(output, expectedOutput)
							}
						})
					})

					when("ctrl+c", func() {
						it("stops the execution", func() {
							var buf = new(bytes.Buffer)
							command := pack.StartWithWriter(
								buf,
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
							)

							go command.TerminateAtStep("DETECTING")

							err := command.Wait()
							assert.NotNil(err)
							assert.NotContain(buf.String(), "Successfully built image")
						})
					})

					when("--descriptor", func() {

						when("exclude and include", func() {
							var buildpackTgz, tempAppDir string

							it.Before(func() {
								h.SkipUnless(t,
									pack.SupportsFeature(invoke.ExcludeAndIncludeDescriptor),
									"pack --descriptor does NOT support 'exclude' and 'include' feature",
								)

								buildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "descriptor-buildpack"), "./", 0755)

								var err error
								tempAppDir, err = ioutil.TempDir("", "descriptor-app")
								assert.Nil(err)

								// Create test directories and files:
								//
								// ├── cookie.jar
								// ├── secrets
								// │   ├── api_keys.json
								// |   |── user_token
								// ├── media
								// │   ├── mountain.jpg
								// │   └── person.png
								// └── test.sh
								err = os.Mkdir(filepath.Join(tempAppDir, "secrets"), 0755)
								assert.Nil(err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "secrets", "api_keys.json"), []byte("{}"), 0755)
								assert.Nil(err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "secrets", "user_token"), []byte("token"), 0755)
								assert.Nil(err)

								err = os.Mkdir(filepath.Join(tempAppDir, "media"), 0755)
								assert.Nil(err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "media", "mountain.jpg"), []byte("fake image bytes"), 0755)
								assert.Nil(err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "media", "person.png"), []byte("fake image bytes"), 0755)
								assert.Nil(err)

								err = ioutil.WriteFile(filepath.Join(tempAppDir, "cookie.jar"), []byte("chocolate chip"), 0755)
								assert.Nil(err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "test.sh"), []byte("echo test"), 0755)
								assert.Nil(err)
							})

							it.After(func() {
								assert.Nil(os.RemoveAll(tempAppDir))
							})

							it("should exclude ALL specified files and directories", func() {
								projectToml := `
[project]
name = "exclude test"
[[project.licenses]]
type = "MIT"
[build]
exclude = [ "*.sh", "secrets/", "media/metadata" ]
`
								excludeDescriptorPath := filepath.Join(tempAppDir, "exclude.toml")
								err := ioutil.WriteFile(excludeDescriptorPath, []byte(projectToml), 0755)
								assert.Nil(err)

								output := pack.RunSuccessfully(
									"build",
									repoName,
									"-p", tempAppDir,
									"--buildpack", buildpackTgz,
									"--descriptor", excludeDescriptorPath,
								)
								assert.NotContain(output, "api_keys.json")
								assert.NotContain(output, "user_token")
								assert.NotContain(output, "test.sh")

								assert.Contains(output, "cookie.jar")
								assert.Contains(output, "mountain.jpg")
								assert.Contains(output, "person.png")
							})

							it("should ONLY include specified files and directories", func() {
								projectToml := `
[project]
name = "include test"
[[project.licenses]]
type = "MIT"
[build]
include = [ "*.jar", "media/mountain.jpg", "media/person.png" ]
`
								includeDescriptorPath := filepath.Join(tempAppDir, "include.toml")
								err := ioutil.WriteFile(includeDescriptorPath, []byte(projectToml), 0755)
								assert.Nil(err)

								output := pack.RunSuccessfully(
									"build",
									repoName,
									"-p", tempAppDir,
									"--buildpack", buildpackTgz,
									"--descriptor", includeDescriptorPath,
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
				})
			})

			when("inspect-builder", func() {
				when("inspecting a nested builder", func() {
					it.Before(func() {
						// create our nested builder
						h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

						h.SkipUnless(t,
							pack.Supports("inspect-builder --depth"),
							"pack does not support 'package-buildpack'",
						)
						// create a task, handled by a 'task manager' which executes our pack commands during tests.
						// looks like this is used to de-dup tasks
						key := taskKey(
							"create-complex-builder",
							append(
								[]string{runImageMirror, createBuilderPackConfig.Path(), lifecycle.Identifier()},
								createBuilderPackConfig.FixturePaths()...,
							)...,
						)
						// run task on taskmanager and save output, in case there are future calls to the same task
						// likely all our changes need to go on the createBuilderPack.
						value, err := suiteManager.RunTaskOnceString(key, func() (string, error) {
							return createComplexBuilder(
								t,
								assert,
								createBuilderPack,
								lifecycle,
								runImageMirror,
							)
						})
						h.AssertNil(t, err)

						// register task to be run to 'clean up' a task
						suiteManager.RegisterCleanUp("clean-"+key, func() error {
							return h.DockerRmi(dockerCli, value)
						})
						builderName = value
					})

					it("displays nested Detection Order groups", func() {
						output := pack.RunSuccessfully(
							"set-run-image-mirrors", "pack-test/run", "--mirror", "some-registry.com/pack-test/run1",
						)
						h.AssertEq(t, output, "Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n")

						output = pack.RunSuccessfully("inspect-builder", builderName)

						deprecatedBuildpackAPIs,
							supportedBuildpackAPIs,
							deprecatedPlatformAPIs,
							supportedPlatformAPIs := lifecycle.OutputForAPIs()

						expectedOutput := pack.FixtureManager().TemplateVersionedFixture(
							"inspect_%s_builder_nested_output.txt",
							createBuilderPack.Version(),
							"inspect_builder_nested_output.txt",
							map[string]interface{}{
								"builder_name":              builderName,
								"lifecycle_version":         lifecycle.Version(),
								"deprecated_buildpack_apis": deprecatedBuildpackAPIs,
								"supported_buildpack_apis":  supportedBuildpackAPIs,
								"deprecated_platform_apis":  deprecatedPlatformAPIs,
								"supported_platform_apis":   supportedPlatformAPIs,
								"run_image_mirror":          runImageMirror,
								"pack_version":              createBuilderPack.Version(),
								"trusted":                   "No",

								// set previous pack template fields
								"buildpack_api_version": lifecycle.EarliestBuildpackAPIVersion(),
								"platform_api_version":  lifecycle.EarliestPlatformAPIVersion(),
							},
						)

						assert.TrimmedEq(output, expectedOutput)
					})

					it("provides nested detection output up to depth", func() {
						output := pack.RunSuccessfully(
							"set-run-image-mirrors", "pack-test/run", "--mirror", "some-registry.com/pack-test/run1",
						)
						assert.Equal(output, "Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n")

						output = pack.RunSuccessfully("inspect-builder", "--depth", "2", builderName)

						deprecatedBuildpackAPIs,
							supportedBuildpackAPIs,
							deprecatedPlatformAPIs,
							supportedPlatformAPIs := lifecycle.OutputForAPIs()

						expectedOutput := pack.FixtureManager().TemplateVersionedFixture(
							"inspect_%s_builder_nested_depth_2_output.txt",
							createBuilderPack.Version(),
							"inspect_builder_nested_depth_2_output.txt",
							map[string]interface{}{
								"builder_name":              builderName,
								"lifecycle_version":         lifecycle.Version(),
								"deprecated_buildpack_apis": deprecatedBuildpackAPIs,
								"supported_buildpack_apis":  supportedBuildpackAPIs,
								"deprecated_platform_apis":  deprecatedPlatformAPIs,
								"supported_platform_apis":   supportedPlatformAPIs,
								"run_image_mirror":          runImageMirror,
								"pack_version":              createBuilderPack.Version(),
								"trusted":                   "No",

								// set previous pack template fields
								"buildpack_api_version": lifecycle.EarliestBuildpackAPIVersion(),
								"platform_api_version":  lifecycle.EarliestPlatformAPIVersion(),
							},
						)

						assert.TrimmedEq(output, expectedOutput)
					})
				})

				it("displays configuration for a builder (local and remote)", func() {
					output := pack.RunSuccessfully(
						"set-run-image-mirrors", "pack-test/run", "--mirror", "some-registry.com/pack-test/run1",
					)
					assert.Equal(output, "Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n")

					output = pack.RunSuccessfully("inspect-builder", builderName)

					deprecatedBuildpackAPIs,
						supportedBuildpackAPIs,
						deprecatedPlatformAPIs,
						supportedPlatformAPIs := lifecycle.OutputForAPIs()

					expectedOutput := pack.FixtureManager().TemplateVersionedFixture(
						"inspect_%s_builder_output.txt",
						createBuilderPack.Version(),
						"inspect_builder_output.txt",
						map[string]interface{}{
							"builder_name":              builderName,
							"lifecycle_version":         lifecycle.Version(),
							"deprecated_buildpack_apis": deprecatedBuildpackAPIs,
							"supported_buildpack_apis":  supportedBuildpackAPIs,
							"deprecated_platform_apis":  deprecatedPlatformAPIs,
							"supported_platform_apis":   supportedPlatformAPIs,
							"run_image_mirror":          runImageMirror,
							"pack_version":              createBuilderPack.Version(),
							"trusted":                   "No",

							// set previous pack template fields
							"buildpack_api_version": lifecycle.EarliestBuildpackAPIVersion(),
							"platform_api_version":  lifecycle.EarliestPlatformAPIVersion(),
						},
					)

					assert.TrimmedEq(output, expectedOutput)
				})

				it("indicates builder is trusted", func() {
					h.SkipUnless(t, pack.Supports("trust-builder"), "version of pack doesn't trust-builder command")

					pack.JustRunSuccessfully("trust-builder", builderName)
					pack.JustRunSuccessfully(
						"set-run-image-mirrors", "pack-test/run", "--mirror", "some-registry.com/pack-test/run1",
					)

					output := pack.RunSuccessfully("inspect-builder", builderName)

					deprecatedBuildpackAPIs,
						supportedBuildpackAPIs,
						deprecatedPlatformAPIs,
						supportedPlatformAPIs := lifecycle.OutputForAPIs()

					expectedOutput := pack.FixtureManager().TemplateVersionedFixture(
						"inspect_%s_builder_output.txt",
						createBuilderPack.Version(),
						"inspect_builder_output.txt",
						map[string]interface{}{
							"builder_name":              builderName,
							"lifecycle_version":         lifecycle.Version(),
							"deprecated_buildpack_apis": deprecatedBuildpackAPIs,
							"supported_buildpack_apis":  supportedBuildpackAPIs,
							"deprecated_platform_apis":  deprecatedPlatformAPIs,
							"supported_platform_apis":   supportedPlatformAPIs,
							"run_image_mirror":          runImageMirror,
							"pack_version":              createBuilderPack.Version(),
							"trusted":                   "Yes",

							// set previous pack template fields
							"buildpack_api_version": lifecycle.EarliestBuildpackAPIVersion(),
							"platform_api_version":  lifecycle.EarliestPlatformAPIVersion(),
						},
					)

					assert.TrimmedEq(output, expectedOutput)
				})
			})

			when("rebase", func() {
				var repoName, runBefore, origID string
				var buildRunImage func(string, string, string)

				it.Before(func() {
					pack.JustRunSuccessfully("trust-builder", builderName)

					repoName = registryConfig.RepoName("some-org/" + h.RandString(10))
					runBefore = registryConfig.RepoName("run-before/" + h.RandString(10))

					buildRunImage = func(newRunImage, contents1, contents2 string) {
						user := func() string {
							if dockerHostOS() == "windows" {
								return "ContainerAdministrator"
							}

							return "root"
						}

						h.CreateImage(t, dockerCli, newRunImage, fmt.Sprintf(`
													FROM %s
													USER %s
													RUN echo %s > /contents1.txt
													RUN echo %s > /contents2.txt
													USER pack
												`, runImage, user(), contents1, contents2))
					}

					buildRunImage(runBefore, "contents-before-1", "contents-before-2")
					// TODO: Replace --no-pull with pull-policy never. See https://github.com/buildpacks/pack/issues/775
					pack.RunSuccessfully(
						"build", repoName,
						"-p", filepath.Join("testdata", "mock_app"),
						"--builder", builderName,
						"--run-image", runBefore,
						"--no-pull",
					)
					origID = h.ImageID(t, repoName)
					assertMockAppRunsWithOutput(t,
						assert,
						repoName,
						"contents-before-1",
						"contents-before-2",
					)
				})

				it.After(func() {
					h.DockerRmi(dockerCli, origID, repoName, runBefore)
					ref, err := name.ParseReference(repoName, name.WeakValidation)
					assert.Nil(err)
					buildCacheVolume := cache.NewVolumeCache(ref, "build", dockerCli)
					launchCacheVolume := cache.NewVolumeCache(ref, "launch", dockerCli)
					assert.Nil(buildCacheVolume.Clear(context.TODO()))
					assert.Nil(launchCacheVolume.Clear(context.TODO()))
				})

				when("daemon", func() {
					when("--run-image", func() {
						var runAfter string

						it.Before(func() {
							runAfter = registryConfig.RepoName("run-after/" + h.RandString(10))
							buildRunImage(runAfter, "contents-after-1", "contents-after-2")
						})

						it.After(func() {
							assert.Nil(h.DockerRmi(dockerCli, runAfter))
						})

						it("uses provided run image", func() {
							// TODO: Replace --no-pull with pull-policy never. See https://github.com/buildpacks/pack/issues/775
							output := pack.RunSuccessfully(
								"rebase", repoName,
								"--run-image", runAfter,
								"--no-pull",
							)

							assert.Contains(output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"contents-after-1",
								"contents-after-2",
							)
						})
					})

					when("local config has a mirror", func() {
						var localRunImageMirror string

						it.Before(func() {
							localRunImageMirror = registryConfig.RepoName("run-after/" + h.RandString(10))
							buildRunImage(localRunImageMirror, "local-mirror-after-1", "local-mirror-after-2")
							pack.JustRunSuccessfully("set-run-image-mirrors", runImage, "-m", localRunImageMirror)
						})

						it.After(func() {
							assert.Nil(h.DockerRmi(dockerCli, localRunImageMirror))
						})

						it("prefers the local mirror", func() {
							// TODO: Replace --no-pull with pull-policy never. See https://github.com/buildpacks/pack/issues/775
							output := pack.RunSuccessfully("rebase", repoName, "--no-pull")

							assertOutput := assertions.NewOutputAssertionManager(t, output)
							assertOutput.ReportsSelectingRunImageMirrorFromLocalConfig(localRunImageMirror)
							assertOutput.ReportsSuccessfulRebase(repoName)
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"local-mirror-after-1",
								"local-mirror-after-2",
							)
						})
					})

					when("image metadata has a mirror", func() {
						it.Before(func() {
							// clean up existing mirror first to avoid leaking images
							assert.Nil(h.DockerRmi(dockerCli, runImageMirror))

							buildRunImage(runImageMirror, "mirror-after-1", "mirror-after-2")
						})

						it("selects the best mirror", func() {
							// TODO: Replace --no-pull with pull-policy never. See https://github.com/buildpacks/pack/issues/775
							output := pack.RunSuccessfully("rebase", repoName, "--no-pull")

							assertOutput := assertions.NewOutputAssertionManager(t, output)
							assertOutput.ReportsSelectingRunImageMirror(runImageMirror)
							assertOutput.ReportsSuccessfulRebase(repoName)
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"mirror-after-1",
								"mirror-after-2",
							)
						})
					})
				})

				when("--publish", func() {
					it.Before(func() {
						assert.Nil(h.PushImage(dockerCli, repoName, registryConfig))
					})

					when("--run-image", func() {
						var runAfter string

						it.Before(func() {
							runAfter = registryConfig.RepoName("run-after/" + h.RandString(10))
							buildRunImage(runAfter, "contents-after-1", "contents-after-2")
							assert.Nil(h.PushImage(dockerCli, runAfter, registryConfig))
						})

						it.After(func() {
							h.DockerRmi(dockerCli, runAfter)
						})

						it("uses provided run image", func() {
							output := pack.RunSuccessfully("rebase", repoName, "--publish", "--run-image", runAfter)

							assertions.NewOutputAssertionManager(t, output).ReportsSuccessfulRebase(repoName)
							assert.Nil(h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
							assertMockAppRunsWithOutput(t,
								assert,
								repoName,
								"contents-after-1",
								"contents-after-2",
							)
						})
					})
				})
			})
		})
	})
}

func buildpacksDir(bpAPIVersion string) string {
	return filepath.Join("testdata", "mock_buildpacks", bpAPIVersion)
}

func createComplexBuilder(t *testing.T,
	assert h.AssertionManager,
	pack *invoke.PackInvoker,
	lifecycle config.LifecycleAsset,
	runImageMirror string) (string, error) {

	t.Log("creating complex builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-complex-test-builder")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(lifecycle.EarliestBuildpackAPIVersion())
	t.Log("using buildpacks from: ", buildpacksDir)
	h.RecursiveCopy(t, buildpacksDir, tmpDir)

	// ARCHIVE BUILDPACKS
	buildpacks := []string{
		"noop-buildpack",
		"noop-buildpack-2",
		"other-stack-buildpack",
		"read-env-buildpack",
	}

	for _, v := range buildpacks {
		tgz := h.CreateTGZ(t, filepath.Join(buildpacksDir, v), "./", 0755)
		err := os.Rename(tgz, filepath.Join(tmpDir, v+".tgz"))
		if err != nil {
			return "", err
		}
	}
	buildpackImages := []string{
		"simple-layers-buildpack",
		"nested-level-2-buildpack",
		"nested-level-1-buildpack",
	}

	buildpackImageToName := map[string]interface{}{}

	var packageImageName string
	var packageId string

	if dockerHostOS() != "windows" {
		for _, buildpackName := range buildpackImages {
			packageFileName := buildpackName + "_package.toml"
			packageFileDest := filepath.Join(tmpDir, packageFileName)
			packageFile, err := os.OpenFile(packageFileDest, os.O_CREATE|os.O_TRUNC|os.O_RDWR, os.ModePerm)
			if err != nil {
				return "", err
			}

			pack.FixtureManager().TemplateFixtureToFile(packageFileName, packageFile, buildpackImageToName)

			packageImageName = packageBuildpackAsImage(t,
				assert,
				pack,
				packageFileDest,
				lifecycle,
				[]string{buildpackName},
			)

			buildpackKey := strings.ReplaceAll(buildpackName, "-", "_")
			buildpackImageToName[buildpackKey] = packageImageName
		}

		packageId = "simple/nested-level-1"
	}

	// ADD lifecycle
	var lifecycleURI string
	var lifecycleVersion string
	if lifecycle.HasLocation() {
		lifecycleURI = lifecycle.EscapedPath()
		t.Logf("adding lifecycle path '%s' to builder config", lifecycleURI)
	} else {
		lifecycleVersion = lifecycle.Version()
		t.Logf("adding lifecycle version '%s' to builder config", lifecycleVersion)
	}

	// RENDER builder.toml
	builderConfigFile, err := ioutil.TempFile(tmpDir, "nested_builder.toml")
	if err != nil {
		return "", err
	}

	templateMapping := map[string]interface{}{
		"package_image_name": packageImageName,
		"package_id":         packageId,
		"run_image_mirror":   runImageMirror,
		"lifecycle_uri":      lifecycleURI,
		"lifecycle_version":  lifecycleVersion,
	}

	for key, val := range buildpackImageToName {
		templateMapping[key] = val
	}

	pack.FixtureManager().TemplateFixtureToFile(
		"nested_builder.toml",
		builderConfigFile,
		templateMapping,
	)

	err = builderConfigFile.Close()
	if err != nil {
		return "", err
	}

	// NAME BUILDER
	bldr := registryConfig.RepoName("test/builder-" + h.RandString(10))

	// CREATE BUILDER
	output, err := pack.Run(
		"create-builder", bldr,
		"-c", builderConfigFile.Name(),
		"--no-color",
	)
	if err != nil {
		return "", errors.Wrapf(err, "pack failed with output %s", output)
	}

	assert.Contains(output, fmt.Sprintf("Successfully created builder image '%s'", bldr))
	assert.Nil(h.PushImage(dockerCli, bldr, registryConfig))

	return bldr, nil
}

func createBuilder(
	t *testing.T,
	assert h.AssertionManager,
	pack *invoke.PackInvoker,
	lifecycle config.LifecycleAsset,
	runImageMirror string,
) (string, error) {
	t.Log("creating builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(lifecycle.EarliestBuildpackAPIVersion())
	t.Log("using buildpacks from: ", buildpacksDir)
	h.RecursiveCopy(t, buildpacksDir, tmpDir)

	// ARCHIVE BUILDPACKS
	buildpacks := []string{
		"noop-buildpack",
		"noop-buildpack-2",
		"other-stack-buildpack",
		"read-env-buildpack",
	}

	// NOTE: Windows-based packages are not yet supported, so we'll add this buildpack in the usual way for now.
	// Remove this block once Windows-based packages are supported.
	if dockerHostOS() == "windows" {
		buildpacks = append(buildpacks, "simple-layers-buildpack")
	}

	for _, v := range buildpacks {
		tgz := h.CreateTGZ(t, filepath.Join(buildpacksDir, v), "./", 0755)
		err := os.Rename(tgz, filepath.Join(tmpDir, v+".tgz"))
		if err != nil {
			return "", err
		}
	}

	var packageImageName string
	var packageId string

	// NOTE: Windows-based packages are not yet supported, so we'll add this buildpack in the usual way for now (see above).
	// Remove this guard once Windows-based packages are supported.
	if dockerHostOS() != "windows" {
		// CREATE PACKAGE
		packageImageName = packageBuildpackAsImage(t,
			assert,
			pack,
			pack.FixtureManager().FixtureLocation("package.toml"),
			lifecycle,
			[]string{"simple-layers-buildpack"},
		)

		packageId = "simple/layers"
	}

	// ADD lifecycle
	var lifecycleURI string
	var lifecycleVersion string
	if lifecycle.HasLocation() {
		lifecycleURI = lifecycle.EscapedPath()
		t.Logf("adding lifecycle path '%s' to builder config", lifecycleURI)
	} else {
		lifecycleVersion = lifecycle.Version()
		t.Logf("adding lifecycle version '%s' to builder config", lifecycleVersion)
	}

	// RENDER builder.toml
	configFileName := "builder.toml"

	// NOTE: Remove when Windows-based packages are supported (can use same toml at that point)
	if dockerHostOS() == "windows" {
		configFileName = "builder-windows.toml"
	}

	builderConfigFile, err := ioutil.TempFile(tmpDir, "builder.toml")
	if err != nil {
		return "", err
	}

	pack.FixtureManager().TemplateFixtureToFile(
		configFileName,
		builderConfigFile,
		map[string]interface{}{
			"package_image_name": packageImageName,
			"package_id":         packageId,
			"run_image_mirror":   runImageMirror,
			"lifecycle_uri":      lifecycleURI,
			"lifecycle_version":  lifecycleVersion,
		},
	)

	err = builderConfigFile.Close()
	if err != nil {
		return "", err
	}

	// NAME BUILDER
	bldr := registryConfig.RepoName("test/builder-" + h.RandString(10))

	// CREATE BUILDER
	output, err := pack.Run(
		"create-builder", bldr,
		"-c", builderConfigFile.Name(),
		"--no-color",
	)
	if err != nil {
		return "", errors.Wrapf(err, "pack failed with output %s", output)
	}
	assert.Contains(output, fmt.Sprintf("Successfully created builder image '%s'", bldr))
	assert.Nil(h.PushImage(dockerCli, bldr, registryConfig))

	return bldr, nil
}

func packageBuildpackAsImage(
	t *testing.T,
	assert h.AssertionManager,
	pack *invoke.PackInvoker, configLocation string,
	lifecycle config.LifecycleAsset,
	buildpacks []string,
) string {
	tmpDir, err := ioutil.TempDir("", "package-image")
	assert.Nil(err)

	outputImage := packageBuildpack(t, assert, pack, configLocation, tmpDir, "image", lifecycle, buildpacks)

	// REGISTER CLEANUP
	key := taskKey("package-buildpack", outputImage)
	suiteManager.RegisterCleanUp("clean-"+key, func() error {
		return h.DockerRmi(dockerCli, outputImage)
	})

	return outputImage
}

func packageBuildpackAsFile(
	t *testing.T,
	assert h.AssertionManager,
	pack *invoke.PackInvoker, configLocation, tmpDir string,
	lifecycle config.LifecycleAsset,
	buildpacks []string,
) string {

	return packageBuildpack(t, assert, pack, configLocation, tmpDir, "file", lifecycle, buildpacks)
}

func packageBuildpack(
	t *testing.T,
	assert h.AssertionManager,
	pack *invoke.PackInvoker,
	configLocation, tmpDir, outputFormat string,
	lifecycle config.LifecycleAsset,
	buildpacks []string,
) string {

	t.Helper()
	t.Log("creating package image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir(tmpDir, "create-package")
	assert.Nil(err)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(lifecycle.EarliestBuildpackAPIVersion())
	t.Log("using buildpacks from: ", buildpacksDir)
	h.RecursiveCopy(t, buildpacksDir, tmpDir)

	// ARCHIVE BUILDPACKS
	for _, v := range buildpacks {
		tgz := h.CreateTGZ(t, filepath.Join(buildpacksDir, v), "./", 0755)
		err := os.Rename(tgz, filepath.Join(tmpDir, v+".tgz"))
		assert.Nil(err)
	}

	packageConfig := filepath.Join(tmpDir, buildpacks[len(buildpacks)-1]+"_package.toml")

	// COPY config to temp package.toml
	h.CopyFile(t, configLocation, packageConfig)

	// CREATE PACKAGE
	outputName := "buildpack-" + h.RandString(8)
	var additionalArgs []string
	switch outputFormat {
	case "file":
		outputName = filepath.Join(tmpDir, outputName+".cnb")
		additionalArgs = []string{"--format", outputFormat}
	case "image":
		outputName = registryConfig.RepoName(outputName)
	default:
		t.Fatalf("unknown format: %s", outputFormat)
	}

	output := pack.RunSuccessfully(
		"package-buildpack",
		append([]string{
			outputName,
			"--no-color",
			"-c", packageConfig,
		}, additionalArgs...)...,
	)

	assert.Contains(output, fmt.Sprintf("Successfully created package '%s'", outputName))

	return outputName
}

func createStack(t *testing.T, dockerCli client.CommonAPIClient, runImageMirror string) error {
	t.Helper()
	t.Log("creating stack images...")

	stackBaseDir := filepath.Join("testdata", "mock_stack", dockerHostOS())

	if err := createStackImage(dockerCli, runImage, filepath.Join(stackBaseDir, "run")); err != nil {
		return err
	}
	if err := createStackImage(dockerCli, buildImage, filepath.Join(stackBaseDir, "build")); err != nil {
		return err
	}

	if err := dockerCli.ImageTag(context.Background(), runImage, runImageMirror); err != nil {
		return err
	}

	if err := h.PushImage(dockerCli, runImageMirror, registryConfig); err != nil {
		return err
	}

	return nil
}

func createStackImage(dockerCli client.CommonAPIClient, repoName string, dir string) error {
	defaultFilterFunc := func(file string) bool { return true }

	ctx := context.Background()
	buildContext := archive.ReadDirAsTar(dir, "/", 0, 0, -1, true, defaultFilterFunc)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(ioutil.Discard, res.Body)
	if err != nil {
		return err
	}

	return res.Body.Close()
}

type logWriter struct {
	t *testing.T
}

func (l logWriter) Write(p []byte) (n int, err error) {
	l.t.Helper()
	l.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func assertMockAppRunsWithOutput(t *testing.T, assert h.AssertionManager, repoName string, expectedOutputs ...string) {
	t.Helper()
	containerName := "test-" + h.RandString(10)
	ctrID := runDockerImageExposePort(t, assert, containerName, repoName)
	defer dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
	defer dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})
	logs, err := dockerCli.ContainerLogs(context.TODO(), ctrID, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	h.AssertNil(t, err)

	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(logWriter{t}, logWriter{t}, logs)
		copyErr <- err
	}()

	launchPort := fetchHostPort(t, assert, containerName)
	assertMockAppResponseContains(t, assert, launchPort, 10*time.Second, expectedOutputs...)
}

func assertMockAppLogs(t *testing.T, assert h.AssertionManager, repoName string, expectedOutputs ...string) {
	t.Helper()
	containerName := "test-" + h.RandString(10)
	ctr, err := dockerCli.ContainerCreate(context.Background(), &container.Config{
		Image: repoName,
	}, nil, nil, containerName)
	assert.Nil(err)

	var b bytes.Buffer
	err = h.RunContainer(context.Background(), dockerCli, ctr.ID, &b, &b)
	assert.Nil(err)

	for _, expectedOutput := range expectedOutputs {
		assert.Contains(b.String(), expectedOutput)
	}
}

func assertMockAppResponseContains(t *testing.T, assert h.AssertionManager, launchPort string, timeout time.Duration, expectedOutputs ...string) {
	t.Helper()
	resp := waitForResponse(t, launchPort, timeout)
	for _, expected := range expectedOutputs {
		assert.Contains(resp, expected)
	}
}

func assertHasBase(t *testing.T, assert h.AssertionManager, image, base string) {
	t.Helper()
	imageInspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), image)
	assert.Nil(err)
	baseInspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), base)
	assert.Nil(err)
	for i, layer := range baseInspect.RootFS.Layers {
		assert.Equal(imageInspect.RootFS.Layers[i], layer)
	}
}

func fetchHostPort(t *testing.T, assert h.AssertionManager, dockerID string) string {
	t.Helper()

	i, err := dockerCli.ContainerInspect(context.Background(), dockerID)
	assert.Nil(err)
	for _, port := range i.NetworkSettings.Ports {
		for _, binding := range port {
			return binding.HostPort
		}
	}

	t.Fatalf("Failed to fetch host port for %s: no ports exposed", dockerID)
	return ""
}

func imgIDForRepoName(repoName string) (string, error) {
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
	if err != nil {
		return "", errors.Wrapf(err, "could not get image ID for image '%s'", repoName)
	}
	return inspect.ID, nil
}

func runDockerImageExposePort(t *testing.T, assert h.AssertionManager, containerName, repoName string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
		Image:        repoName,
		ExposedPorts: map[nat.Port]struct{}{"8080/tcp": {}},
		Healthcheck:  nil,
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{{}},
		},
		AutoRemove: true,
	}, nil, containerName)
	assert.Nil(err)

	err = dockerCli.ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	assert.Nil(err)
	return ctr.ID
}

func waitForResponse(t *testing.T, port string, timeout time.Duration) string {
	t.Helper()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := h.HTTPGetE("http://"+h.RegistryHost(h.DockerHostname(t), port), map[string]string{})
			if err != nil {
				break
			}
			return resp
		case <-timer.C:
			t.Fatalf("timeout waiting for response: %v", timeout)
		}
	}
}

func imageLabel(t *testing.T, assert h.AssertionManager, dockerCli client.CommonAPIClient, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	assert.Nil(err)
	label, ok := inspect.Config.Labels[labelName]
	if !ok {
		t.Errorf("expected label %s to exist", labelName)
	}
	return label
}

func dockerHostOS() string {
	daemonInfo, err := dockerCli.Info(context.TODO())
	if err != nil {
		panic(err.Error())
	}
	return daemonInfo.OSType
}

// taskKey creates a key from the prefix and all arguments to be unique
func taskKey(prefix string, args ...string) string {
	hash := sha256.New()
	for _, v := range args {
		hash.Write([]byte(v))
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(hash.Sum(nil)))
}
