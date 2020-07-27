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

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/internal/cache"
	"github.com/buildpacks/pack/internal/style"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/acceptance/config"
	"github.com/buildpacks/pack/acceptance/invoke"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/builder"
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

	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	registryConfig = h.RunRegistry(t)
	defer registryConfig.StopRegistry(t)

	inputConfigManager, err := config.NewInputConfigurationManager()
	h.AssertNil(t, err)

	assetsConfig := config.ConvergedAssetManager(t, inputConfigManager)

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

	h.AssertNil(t, suiteManager.CleanUp())
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
		bpDir = buildpacksDir(builder.DefaultBuildpackAPIVersion)
		pack  *invoke.PackInvoker
	)

	it.Before(func() {
		pack = invoke.NewPackInvoker(t, packConfig, registryConfig.DockerConfigDir)
	})

	it.After(func() {
		pack.Cleanup()
	})

	when("invalid subcommand", func() {
		it("prints usage", func() {
			output, err := pack.Run("some-bad-command")
			h.AssertNotNil(t, err)
			h.AssertContains(t, output, `unknown command "some-bad-command" for "pack"`)
			h.AssertContains(t, output, `Run 'pack --help' for usage.`)
		})
	})

	when("suggest-builders", func() {
		it("displays suggested builders", func() {
			output := pack.RunSuccessfully("suggest-builders")

			h.AssertContains(t, output, "Suggested builders:")
			h.AssertMatch(t, output, `Google:\s+'gcr.io/buildpacks/builder:v1'`)
			h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks:18'`)
			h.AssertMatch(t, output, `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:base'`)
			h.AssertMatch(t, output, `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:full-cf'`)
			h.AssertMatch(t, output, `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:tiny'`)
		})
	})

	when("suggest-stacks", func() {
		it("displays suggested stacks", func() {
			output, err := pack.Run("suggest-stacks")
			if err != nil {
				t.Fatalf("suggest-stacks command failed: %s: %s", output, err)
			}
			h.AssertContains(t, string(output), "Stacks maintained by the community:")
		})
	})

	when("set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			output := pack.RunSuccessfully("set-default-builder", "gcr.io/paketo-buildpacks/builder:base")
			h.AssertContains(t, output, "Builder 'gcr.io/paketo-buildpacks/builder:base' is now the default builder")
		})
	})

	when("trust-builder", func() {
		it("sets the builder as trusted in ~/.pack/config.toml", func() {
			h.SkipIf(t, !pack.Supports("trust-builder"), "pack does not support 'trust-builder'")
			builderName := "some-builder" + h.RandString(10)

			pack.JustRunSuccessfully("trust-builder", builderName)

			h.AssertContains(t, pack.ConfigFileContents(), builderName)
		})
	})

	when("untrust-builder", func() {
		it("removes the previously trusted builder from ~/${PACK_HOME}/config.toml", func() {
			h.SkipIf(t, !pack.Supports("untrust-builder"), "pack does not support 'untrust-builder'")
			builderName := "some-builder" + h.RandString(10)

			pack.JustRunSuccessfully("trust-builder", builderName)

			h.AssertContains(t, pack.ConfigFileContents(), builderName)

			pack.JustRunSuccessfully("untrust-builder", builderName)

			h.AssertNotContains(t, pack.ConfigFileContents(), builderName)
		})
	})

	when("list-trusted-builders", func() {
		it.Before(func() {
			h.SkipIf(t,
				!pack.Supports("list-trusted-builders"),
				"pack does not support 'list-trusted-builders",
			)
		})

		it("shows default builders from pack suggest-builders", func() {
			output := pack.RunSuccessfully("list-trusted-builders")

			h.AssertContains(t, output, "Trusted Builders:")
			h.AssertContains(t, output, "gcr.io/buildpacks/builder:v1")
			h.AssertContains(t, output, "heroku/buildpacks:18")
			h.AssertContains(t, output, "gcr.io/paketo-buildpacks/builder:base")
			h.AssertContains(t, output, "gcr.io/paketo-buildpacks/builder:full-cf")
			h.AssertContains(t, output, "gcr.io/paketo-buildpacks/builder:tiny")
		})

		it("shows a builder trusted by pack trust-builder", func() {
			builderName := "some-builder" + h.RandString(10)

			pack.JustRunSuccessfully("trust-builder", builderName)

			output := pack.RunSuccessfully("list-trusted-builders")
			h.AssertContains(t, output, builderName)
		})
	})

	when("package-buildpack", func() {
		var (
			tmpDir                  string
			simplePackageConfigPath string
		)

		it.Before(func() {
			h.SkipIf(t,
				!pack.Supports("package-buildpack"),
				"pack does not support 'package-buildpack'",
			)

			h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

			var err error
			tmpDir, err = ioutil.TempDir("", "package-buildpack-tests")
			h.AssertNil(t, err)

			simplePackageConfigPath = filepath.Join(tmpDir, "package.toml")
			h.CopyFile(t, pack.FixtureManager().FixtureLocation("package.toml"), simplePackageConfigPath)

			err = os.Rename(
				h.CreateTGZ(t, filepath.Join(bpDir, "simple-layers-parent-buildpack"), "./", 0755),
				filepath.Join(tmpDir, "simple-layers-parent-buildpack.tgz"),
			)
			h.AssertNil(t, err)

			err = os.Rename(
				h.CreateTGZ(t, filepath.Join(bpDir, "simple-layers-buildpack"), "./", 0755),
				filepath.Join(tmpDir, "simple-layers-buildpack.tgz"),
			)
			h.AssertNil(t, err)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		packageBuildpackLocally := func(absConfigPath string) string {
			t.Helper()
			packageName := "test/package-" + h.RandString(10)
			output, err := pack.Run("package-buildpack", packageName, "-c", absConfigPath)
			h.AssertNil(t, err)
			h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", packageName))
			return packageName
		}

		packageBuildpackRemotely := func(absConfigPath string) string {
			t.Helper()
			packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
			output, err := pack.Run("package-buildpack", packageName, "-c", absConfigPath, "--publish")
			h.AssertNil(t, err)
			h.AssertContains(t, output, fmt.Sprintf("Successfully published package '%s'", packageName))
			return packageName
		}

		assertImageExistsLocally := func(name string) {
			t.Helper()
			_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), name)
			h.AssertNil(t, err)

		}

		generateAggregatePackageToml := func(buildpackURI, nestedPackageName string) string {
			t.Helper()
			packageTomlFile, err := ioutil.TempFile(tmpDir, "package_aggregate-*.toml")
			h.AssertNil(t, err)

			pack.FixtureManager().TemplateFixtureToFile(
				"package_aggregate.toml",
				packageTomlFile,
				map[string]interface{}{
					"BuildpackURI": buildpackURI,
					"PackageName":  nestedPackageName,
				},
			)

			h.AssertNil(t, packageTomlFile.Close())

			return packageTomlFile.Name()
		}

		when("no --format is provided", func() {
			it("creates the package as image", func() {
				packageName := "test/package-" + h.RandString(10)
				output := pack.RunSuccessfully("package-buildpack", packageName, "-c", simplePackageConfigPath)
				h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", packageName))
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
					h.AssertContains(t, output, fmt.Sprintf("Successfully published package '%s'", packageName))

					_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					h.AssertError(t, err, "No such image")

					h.AssertNil(t, h.PullImageWithAuth(dockerCli, packageName, registryConfig.RegistryAuth()))

					_, _, err = dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					h.AssertNil(t, err)
				})
			})

			when("--no-pull", func() {
				it("should use local image", func() {
					nestedPackage := packageBuildpackLocally(simplePackageConfigPath)
					defer h.DockerRmi(dockerCli, nestedPackage)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackage)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					pack.JustRunSuccessfully(
						"package-buildpack", packageName,
						"-c", aggregatePackageToml,
						"--no-pull",
					)

					_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					h.AssertNil(t, err)

				})

				it("should not pull image from registry", func() {
					nestedPackage := packageBuildpackRemotely(simplePackageConfigPath)
					defer h.DockerRmi(dockerCli, nestedPackage)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackage)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					output, err := pack.Run(
						"package-buildpack", packageName,
						"-c", aggregatePackageToml,
						"--no-pull",
					)
					h.AssertNotNil(t, err)
					h.AssertContains(t,
						output,
						fmt.Sprintf("image '%s' does not exist on the daemon", nestedPackage),
					)
				})
			})
		})

		when("--format file", func() {
			it.Before(func() {
				h.SkipIf(t, !pack.Supports("package-buildpack --format"), "format not supported")
			})

			it("creates the package", func() {
				outputFile := filepath.Join(tmpDir, "package.cnb")
				output, err := pack.Run(
					"package-buildpack", outputFile,
					"--format", "file",
					"-c", simplePackageConfigPath,
				)
				h.AssertNil(t, err)
				h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", outputFile))
				h.AssertTarball(t, outputFile)
			})
		})

		when("package.toml is invalid", func() {
			it("displays an error", func() {
				output, err := pack.Run(
					"package-buildpack", "some-package",
					"-c", pack.FixtureManager().FixtureLocation("invalid_package.toml"),
				)
				h.AssertNotNil(t, err)
				h.AssertContains(t, output, "reading config")
			})
		})
	})

	when("report", func() {
		it.Before(func() {
			h.SkipIf(t, !pack.Supports("report"), "pack does not support 'report' command")
		})

		when("default builder is set", func() {
			it("outputs information", func() {
				pack.RunSuccessfully("set-default-builder", "gcr.io/paketo-buildpacks/builder:base")

				output := pack.RunSuccessfully("report")

				version := pack.Version()

				expectedOutput := pack.FixtureManager().TemplateFixture(
					"report_output.txt",
					map[string]interface{}{
						"DefaultBuilder": "gcr.io/paketo-buildpacks/builder:base",
						"Version":        version,
						"OS":             runtime.GOOS,
						"Arch":           runtime.GOARCH,
					},
				)
				h.AssertEq(t, output, expectedOutput)
			})
		})
	})

	when("build with default builders not set", func() {
		it("informs the user", func() {
			output, err := pack.Run(
				"build", "some/image",
				"-p", filepath.Join("testdata", "mock_app"),
			)

			h.AssertNotNil(t, err)
			h.AssertContains(t, output, `Please select a default builder with:`)
			h.AssertMatch(t, output, `Google:\s+'gcr.io/buildpacks/builder:v1'`)
			h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks:18'`)
			h.AssertMatch(t, output, `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:base'`)
			h.AssertMatch(t, output, `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:full-cf'`)
			h.AssertMatch(t, output, `Paketo Buildpacks:\s+'gcr.io/paketo-buildpacks/builder:tiny'`)
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
		bpDir                   = buildpacksDir(lifecycle.BuildpackAPIVersion())
	)

	it.Before(func() {
		pack = invoke.NewPackInvoker(t, subjectPackConfig, registryConfig.DockerConfigDir)
		createBuilderPack = invoke.NewPackInvoker(t, createBuilderPackConfig, registryConfig.DockerConfigDir)
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
			h.AssertNil(t, err)

			suiteManager.RegisterCleanUp("remove-stack-images", func() error {
				return h.DockerRmi(dockerCli, runImage, buildImage, value)
			})

			runImageMirror = value
		})

		when("creating a windows builder", func() {
			it.Before(func() {
				h.SkipIf(t, dockerHostOS() != "windows", "The current Docker daemon does not support Windows-based containers")
			})

			when("experimental is disabled", func() {
				it("fails", func() {
					builderName, err := createBuilder(t,
						createBuilderPack,
						lifecycle,
						runImageMirror,
					)

					if err != nil {
						defer h.DockerRmi(dockerCli, builderName)
						h.AssertError(t, err, "Windows containers support is currently experimental")
					}
				})
			})

			when("experimental is enabled", func() {
				it("succeeds", func() {
					createBuilderPack.EnableExperimental()

					builderName, err := createBuilder(t, createBuilderPack, lifecycle, runImageMirror)
					h.AssertNil(t, err)
					defer h.DockerRmi(dockerCli, builderName)

					inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), builderName)
					h.AssertNil(t, err)

					h.AssertEq(t, inspect.Os, "windows")
				})
			})
		})

		when("builder is created", func() {
			var (
				builderName string
				tmpDir      string
			)

			it.Before(func() {
				h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

				var err error
				tmpDir, err = ioutil.TempDir("", "package-buildpack-tests")
				h.AssertNil(t, err)

				key := taskKey(
					"create-builder",
					append(
						[]string{runImageMirror, createBuilderPackConfig.Path(), lifecycle.Identifier()},
						createBuilderPackConfig.FixturePaths()...,
					)...,
				)
				value, err := suiteManager.RunTaskOnceString(key, func() (string, error) {
					return createBuilder(t, createBuilderPack, lifecycle, runImageMirror)
				})
				h.AssertNil(t, err)
				suiteManager.RegisterCleanUp("clean-"+key, func() error {
					return h.DockerRmi(dockerCli, value)
				})

				builderName = value
			})

			it.After(func() {
				h.AssertNil(t, os.RemoveAll(tmpDir))
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
					h.AssertNotNil(t, err)
					h.AssertContains(t, output, "invalid builder toml")
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
					h.AssertNil(t, err)
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
						var err error
						untrustedBuilderName, err = createBuilder(
							t,
							createBuilderPack,
							lifecycle,
							runImageMirror,
						)
						h.AssertNil(t, err)
					})

					it.After(func() {
						h.DockerRmi(dockerCli, untrustedBuilderName)
					})

					it("uses the 5 phases", func() {
						output := pack.RunSuccessfully(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"-B", untrustedBuilderName,
						)

						if pack.SupportsFeature(invoke.CreatorInPack) {
							h.AssertContains(t, output, "buildpacksio/lifecycle")
						}
						h.AssertContains(t, output, "[detector]")
						h.AssertContains(t, output, "[analyzer]")
						h.AssertContains(t, output, "[builder]")
						h.AssertContains(t, output, "[exporter]")
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
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

						output := pack.RunSuccessfully("build", repoName, "-p", appPath)

						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
						imgId, err := imgIDForRepoName(repoName)
						if err != nil {
							t.Fatal(err)
						}
						defer h.DockerRmi(dockerCli, imgId)

						t.Log("uses a build cache volume")
						h.AssertContains(t, output, "Using build cache volume")

						t.Log("app is runnable")
						assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

						t.Log("selects the best run image mirror")
						h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s'", runImageMirror))

						t.Log("it uses the run image as a base image")
						assertHasBase(t, repoName, runImage)

						t.Log("sets the run image metadata")
						appMetadataLabel := imageLabel(t, dockerCli, repoName, "io.buildpacks.lifecycle.metadata")
						h.AssertContains(t, appMetadataLabel, fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImage, runImageMirror))

						t.Log("registry is empty")
						contents, err := registryConfig.RegistryCatalog()
						h.AssertNil(t, err)
						if strings.Contains(contents, repo) {
							t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
						}

						t.Log("add a local mirror")
						localRunImageMirror := registryConfig.RepoName("pack-test/run-mirror")
						h.AssertNil(t, dockerCli.ImageTag(context.TODO(), runImage, localRunImageMirror))
						defer h.DockerRmi(dockerCli, localRunImageMirror)
						pack.JustRunSuccessfully("set-run-image-mirrors", runImage, "-m", localRunImageMirror)

						t.Log("rebuild")
						output = pack.RunSuccessfully("build", repoName, "-p", appPath)
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

						imgId, err = imgIDForRepoName(repoName)
						if err != nil {
							t.Fatal(err)
						}
						defer h.DockerRmi(dockerCli, imgId)

						t.Log("local run-image mirror is selected")
						h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s' from local config", localRunImageMirror))

						t.Log("app is runnable")
						assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

						t.Log("restores the cache")
						h.AssertContainsMatch(t, output, `(?i)Restoring data for "simple/layers:cached-launch-layer" from cache`)
						h.AssertContainsMatch(t, output, `(?i)Restoring metadata for "simple/layers:cached-launch-layer" from app image`)

						t.Log("exporter reuses unchanged layers")
						h.AssertContainsMatch(t, output, `(?i)Reusing layer 'simple/layers:cached-launch-layer'`)

						t.Log("cacher reuses unchanged layers")
						h.AssertContainsMatch(t, output, `(?i)Reusing cache layer 'simple/layers:cached-launch-layer'`)

						t.Log("rebuild with --clear-cache")
						output = pack.RunSuccessfully("build", repoName, "-p", appPath, "--clear-cache")
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

						t.Log("skips restore")
						if !usingCreator {
							h.AssertContains(t, output, "Skipping 'restore' due to clearing cache")
						}

						t.Log("skips buildpack layer analysis")
						h.AssertContainsMatch(t, output, `(?i)Skipping buildpack layer analysis`)

						t.Log("exporter reuses unchanged layers")
						h.AssertContainsMatch(t, output, `(?i)Reusing layer 'simple/layers:cached-launch-layer'`)

						t.Log("cacher adds layers")
						h.AssertContainsMatch(t, output, `(?i)Adding cache layer 'simple/layers:cached-launch-layer'`)

						if pack.Supports("inspect-image") {
							t.Log("inspect-image")
							output = pack.RunSuccessfully("inspect-image", repoName)

							expectedOutput := pack.FixtureManager().TemplateFixture(
								"inspect_image_local_output.txt",
								map[string]interface{}{
									"image_name":             repoName,
									"base_image_id":          h.ImageID(t, runImageMirror),
									"base_image_top_layer":   h.TopLayerDiffID(t, runImageMirror),
									"run_image_local_mirror": localRunImageMirror,
									"run_image_mirror":       runImageMirror,
									"show_reference":         lifecycle.ShouldShowReference(),
									"show_processes":         lifecycle.ShouldShowProcesses(),
								},
							)

							h.AssertEq(t, output, expectedOutput)
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

							h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							imgId, err := imgIDForRepoName(repoName)
							if err != nil {
								t.Fatal(err)
							}
							defer h.DockerRmi(dockerCli, imgId)

							t.Log("has no color with --no-color")
							colorCodeMatcher := `\x1b\[[0-9;]*m`
							h.AssertNotContainsMatch(t, output, colorCodeMatcher)
						})
					})

					it("supports building app from a zip file", func() {
						appPath := filepath.Join("testdata", "mock_app.zip")
						output := pack.RunSuccessfully("build", repoName, "-p", appPath)
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

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
							h.AssertNil(t, os.Remove(buildpackTgz))
							h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
						})

						when("the network mode is not provided", func() {
							it("reports that build and detect are online", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
								)

								h.AssertContains(t, output, "RESULT: Connected to the internet")
							})
						})

						when("the network mode is set to default", func() {
							it("reports that build and detect are online", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
								)

								h.AssertContains(t, output, "RESULT: Connected to the internet")
							})
						})

						when("the network mode is set to none", func() {
							it("reports that build and detect are offline", func() {
								output := pack.RunSuccessfully(
									"build",
									repoName,
									"-p",
									filepath.Join("testdata", "mock_app"),
									"--buildpack",
									buildpackTgz,
									"--network",
									"none",
								)

								h.AssertContains(t, output, "RESULT: Disconnected from the internet")
							})
						})
					})

					when("--volume", func() {
						var buildpackTgz, tempVolume string

						it.Before(func() {
							h.SkipUnless(t,
								pack.SupportsFeature(invoke.CustomVolumeMounts),
								"pack 0.11.0 shipped with a volume mounting bug",
							)

							buildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "volume-buildpack"), "./", 0755)

							var err error
							tempVolume, err = ioutil.TempDir("", "my-volume-mount-source")
							h.AssertNil(t, err)
							h.AssertNil(t, os.Chmod(tempVolume, 0755)) // Override umask

							// Some OSes (like macOS) use symlinks for the standard temp dir.
							// Resolve it so it can be properly mounted by the Docker daemon.
							tempVolume, err = filepath.EvalSymlinks(tempVolume)
							h.AssertNil(t, err)

							err = ioutil.WriteFile(filepath.Join(tempVolume, "some-file"), []byte("some-string\n"), 0755)
							h.AssertNil(t, err)
						})

						it.After(func() {
							_ = os.Remove(buildpackTgz)
							_ = h.DockerRmi(dockerCli, repoName)

							_ = os.RemoveAll(tempVolume)
						})

						it("mounts the provided volume in the detect and build phases", func() {
							output := pack.RunSuccessfully(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--buildpack", buildpackTgz,
								"--volume", fmt.Sprintf("%s:%s", tempVolume, "/my-volume-mount-target"),
							)

							if pack.SupportsFeature(invoke.ReadFromVolumeInDetect) {
								h.AssertContains(t, output, "Detect: Reading file '/platform/my-volume-mount-target/some-file': some-string")
							}
							h.AssertContains(t, output, "Build: Reading file '/platform/my-volume-mount-target/some-file': some-string")
						})
					})

					when("--default-process", func() {
						it("sets the default process from those in the process list", func() {
							h.SkipUnless(t, pack.Supports("build --default-process"), "--default-process flag is not supported")
							h.SkipUnless(t,
								lifecycle.SupportsFeature(config.DefaultProcess),
								"skipping default process. Lifecycle does not support it",
							)

							pack.RunSuccessfully(
								"build", repoName,
								"--default-process", "hello",
								"-p", filepath.Join("testdata", "mock_app"),
							)

							assertMockAppLogs(t, repoName, "hello world")
						})
					})

					when("--buildpack", func() {
						when("the argument is an ID", func() {
							it("adds the buildpacks to the builder if necessary and runs them", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", "simple/layers", // Can omit version if only one
									"--buildpack", "noop.buildpack@noop.buildpack.version",
								)

								h.AssertContains(t, output, "Build: Simple Layers Buildpack")
								h.AssertContains(t, output, "Build: NOOP Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

								t.Log("app is runnable")
								assertMockAppRunsWithOutput(t, repoName,
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
								h.AssertNil(t, os.Remove(localBuildpackTgz))
							})

							it("adds the buildpack to the builder and runs it", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", localBuildpackTgz,
								)

								h.AssertContains(t, output, "Adding buildpack 'local/bp' version 'local-bp-version' to builder")
								h.AssertContains(t, output, "Build: Local Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
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

								h.AssertContains(t, output, "Adding buildpack 'local/bp' version 'local-bp-version' to builder")
								h.AssertContains(t, output, "Build: Local Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							})
						})

						when("the argument is a buildpackage image", func() {
							var packageImageName string

							it.Before(func() {
								h.SkipIf(t,
									!pack.Supports("package-buildpack"),
									"--buildpack does not accept buildpackage unless package-buildpack is supported",
								)

								packageImageName = packageBuildpackAsImage(t,
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

								h.AssertContains(t, output, "Adding buildpack 'simple/layers/parent' version 'simple-layers-parent-version' to builder")
								h.AssertContains(t, output, "Adding buildpack 'simple/layers' version 'simple-layers-version' to builder")
								h.AssertContains(t, output, "Build: Simple Layers Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
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

								var err error
								tmpDir, err = ioutil.TempDir("", "package-file")
								h.AssertNil(t, err)

								packageFile = packageBuildpackAsFile(t,
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
								h.AssertNil(t, os.RemoveAll(tmpDir))
							})

							it("adds the buildpacks to the builder and runs them", func() {
								output := pack.RunSuccessfully(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", packageFile,
								)

								h.AssertContains(t, output, "Adding buildpack 'simple/layers/parent' version 'simple-layers-parent-version' to builder")
								h.AssertContains(t, output, "Adding buildpack 'simple/layers' version 'simple-layers-version' to builder")
								h.AssertContains(t, output, "Build: Simple Layers Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							})
						})

						when("the buildpack stack doesn't match the builder", func() {
							var otherStackBuilderTgz string

							it.Before(func() {
								otherStackBuilderTgz = h.CreateTGZ(t, filepath.Join(bpDir, "other-stack-buildpack"), "./", 0755)
							})

							it.After(func() {
								h.AssertNil(t, os.Remove(otherStackBuilderTgz))
							})

							it("errors", func() {
								txt, err := pack.Run(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", otherStackBuilderTgz,
								)

								h.AssertNotNil(t, err)
								h.AssertContains(t, txt, "other/stack/bp")
								h.AssertContains(t, txt, "other-stack-version")
								h.AssertContains(t, txt, "does not support stack 'pack.test.stack'")
							})
						})
					})

					when("--env-file", func() {
						var envPath string

						it.Before(func() {
							envfile, err := ioutil.TempFile("", "envfile")
							h.AssertNil(t, err)
							defer envfile.Close()

							err = os.Setenv("ENV2_CONTENTS", "Env2 Layer Contents From Environment")
							h.AssertNil(t, err)
							envfile.WriteString(`
            DETECT_ENV_BUILDPACK="true"
			ENV1_CONTENTS="Env1 Layer Contents From File"
			ENV2_CONTENTS
			`)
							envPath = envfile.Name()
						})

						it.After(func() {
							h.AssertNil(t, os.Unsetenv("ENV2_CONTENTS"))
							h.AssertNil(t, os.RemoveAll(envPath))
						})

						it("provides the env vars to the build and detect steps", func() {
							output := pack.RunSuccessfully(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--env-file", envPath,
							)

							h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							assertMockAppRunsWithOutput(t,
								repoName,
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
							h.AssertNil(t, os.Unsetenv("ENV2_CONTENTS"))
						})

						it("provides the env vars to the build and detect steps", func() {
							output := pack.RunSuccessfully(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--env", "DETECT_ENV_BUILDPACK=true",
								"--env", `ENV1_CONTENTS="Env1 Layer Contents From Command Line"`,
								"--env", "ENV2_CONTENTS",
							)

							h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							assertMockAppRunsWithOutput(t, repoName, "Env2 Layer Contents From Environment", "Env1 Layer Contents From Command Line")
						})
					})

					when("--run-image", func() {
						var runImageName string

						when("the run-image has the correct stack ID", func() {
							it.Before(func() {
								runImageName = h.CreateImageOnRemote(t, dockerCli, registryConfig, "custom-run-image"+h.RandString(10), fmt.Sprintf(`
													FROM %s
													USER root
													RUN echo "custom-run" > /custom-run.txt
													USER pack
												`, runImage))
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
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

								t.Log("app is runnable")
								assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

								t.Log("pulls the run image")
								h.AssertContains(t, output, fmt.Sprintf("Pulling image '%s'", runImageName))

								t.Log("uses the run image as the base image")
								assertHasBase(t, repoName, runImageName)
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
								txt, err := pack.Run(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--run-image", runImageName,
								)
								h.AssertNotNil(t, err)
								h.AssertContains(t, txt, "run-image stack id 'other.stack.id' does not match builder stack 'pack.test.stack'")
							})
						})
					})

					when("--publish", func() {
						it("creates image on the registry", func() {
							output := pack.RunSuccessfully(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--publish",
								"--network", "host",
							)
							h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

							t.Log("checking that registry has contents")
							contents, err := registryConfig.RegistryCatalog()
							h.AssertNil(t, err)
							if !strings.Contains(contents, repo) {
								t.Fatalf("Expected to see image %s in %s", repo, contents)
							}

							h.AssertNil(t, h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
							defer h.DockerRmi(dockerCli, repoName)

							t.Log("app is runnable")
							assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

							if pack.Supports("inspect-image") {
								t.Log("inspect-image")
								output = pack.RunSuccessfully("inspect-image", repoName)

								expectedOutput := pack.FixtureManager().TemplateFixture(
									"inspect_image_published_output.txt",
									map[string]interface{}{
										"image_name":           repoName,
										"base_image_ref":       strings.Join([]string{runImageMirror, h.Digest(t, runImageMirror)}, "@"),
										"base_image_top_layer": h.TopLayerDiffID(t, runImageMirror),
										"run_image_mirror":     runImageMirror,
										"show_reference":       lifecycle.ShouldShowReference(),
										"show_processes":       lifecycle.ShouldShowReference(),
									},
								)

								h.AssertEq(t, output, expectedOutput)
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
							h.AssertNotNil(t, err)
							h.AssertNotContains(t, buf.String(), "Successfully built image")
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
								h.AssertNil(t, err)

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
								h.AssertNil(t, err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "secrets", "api_keys.json"), []byte("{}"), 0755)
								h.AssertNil(t, err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "secrets", "user_token"), []byte("token"), 0755)
								h.AssertNil(t, err)

								err = os.Mkdir(filepath.Join(tempAppDir, "media"), 0755)
								h.AssertNil(t, err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "media", "mountain.jpg"), []byte("fake image bytes"), 0755)
								h.AssertNil(t, err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "media", "person.png"), []byte("fake image bytes"), 0755)
								h.AssertNil(t, err)

								err = ioutil.WriteFile(filepath.Join(tempAppDir, "cookie.jar"), []byte("chocolate chip"), 0755)
								h.AssertNil(t, err)
								err = ioutil.WriteFile(filepath.Join(tempAppDir, "test.sh"), []byte("echo test"), 0755)
								h.AssertNil(t, err)
							})

							it.After(func() {
								h.AssertNil(t, os.RemoveAll(tempAppDir))
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
								h.AssertNil(t, err)

								output := pack.RunSuccessfully(
									"build",
									repoName,
									"-p", tempAppDir,
									"--buildpack", buildpackTgz,
									"--descriptor", excludeDescriptorPath,
								)
								h.AssertNotContains(t, output, "api_keys.json")
								h.AssertNotContains(t, output, "user_token")
								h.AssertNotContains(t, output, "test.sh")

								h.AssertContains(t, output, "cookie.jar")
								h.AssertContains(t, output, "mountain.jpg")
								h.AssertContains(t, output, "person.png")
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
								h.AssertNil(t, err)

								output := pack.RunSuccessfully(
									"build",
									repoName,
									"-p", tempAppDir,
									"--buildpack", buildpackTgz,
									"--descriptor", includeDescriptorPath,
								)
								h.AssertNotContains(t, output, "api_keys.json")
								h.AssertNotContains(t, output, "user_token")
								h.AssertNotContains(t, output, "test.sh")

								h.AssertContains(t, output, "cookie.jar")
								h.AssertContains(t, output, "mountain.jpg")
								h.AssertContains(t, output, "person.png")
							})
						})
					})
				})
			})

			when("inspect-builder", func() {
				it("displays configuration for a builder (local and remote)", func() {
					output := pack.RunSuccessfully(
						"set-run-image-mirrors", "pack-test/run", "--mirror", "some-registry.com/pack-test/run1",
					)
					h.AssertEq(t, output, "Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n")

					output = pack.RunSuccessfully("inspect-builder", builderName)

					expectedOutput := pack.FixtureManager().TemplateVersionedFixture(
						"inspect_%s_builder_output.txt",
						createBuilderPack.Version(),
						"inspect_builder_output.txt",
						map[string]interface{}{
							"builder_name":          builderName,
							"lifecycle_version":     lifecycle.Version(),
							"buildpack_api_version": lifecycle.BuildpackAPIVersion(),
							"platform_api_version":  lifecycle.PlatformAPIVersion(),
							"run_image_mirror":      runImageMirror,
							"pack_version":          createBuilderPack.Version(),
							"trusted":               "No",
						},
					)

					h.AssertEq(t, output, expectedOutput)
				})

				it("indicates builder is trusted", func() {
					h.SkipUnless(t, pack.Supports("trust-builder"), "version of pack doesn't trust-builder command")

					pack.JustRunSuccessfully("trust-builder", builderName)
					pack.JustRunSuccessfully(
						"set-run-image-mirrors", "pack-test/run", "--mirror", "some-registry.com/pack-test/run1",
					)

					output := pack.RunSuccessfully("inspect-builder", builderName)

					expectedOutput := pack.FixtureManager().TemplateVersionedFixture(
						"inspect_%s_builder_output.txt",
						createBuilderPack.Version(),
						"inspect_builder_output.txt",
						map[string]interface{}{
							"builder_name":          builderName,
							"lifecycle_version":     lifecycle.Version(),
							"buildpack_api_version": lifecycle.BuildpackAPIVersion(),
							"platform_api_version":  lifecycle.PlatformAPIVersion(),
							"run_image_mirror":      runImageMirror,
							"pack_version":          createBuilderPack.Version(),
							"trusted":               "Yes",
						},
					)

					h.AssertEq(t, output, expectedOutput)
				})
			})

			when("rebase", func() {
				var repoName, runBefore, origID string
				var buildRunImage func(string, string, string)

				it.Before(func() {
					repoName = registryConfig.RepoName("some-org/" + h.RandString(10))
					runBefore = registryConfig.RepoName("run-before/" + h.RandString(10))

					buildRunImage = func(newRunImage, contents1, contents2 string) {
						h.CreateImage(t, dockerCli, newRunImage, fmt.Sprintf(`
													FROM %s
													USER root
													RUN echo %s > /contents1.txt
													RUN echo %s > /contents2.txt
													USER pack
												`, runImage, contents1, contents2))
					}
					buildRunImage(runBefore, "contents-before-1", "contents-before-2")
					pack.RunSuccessfully(
						"build", repoName,
						"-p", filepath.Join("testdata", "mock_app"),
						"--builder", builderName,
						"--run-image", runBefore,
						"--no-pull",
					)
					origID = h.ImageID(t, repoName)
					assertMockAppRunsWithOutput(t, repoName, "contents-before-1", "contents-before-2")
				})

				it.After(func() {
					h.DockerRmi(dockerCli, origID, repoName, runBefore)
					ref, err := name.ParseReference(repoName, name.WeakValidation)
					h.AssertNil(t, err)
					buildCacheVolume := cache.NewVolumeCache(ref, "build", dockerCli)
					launchCacheVolume := cache.NewVolumeCache(ref, "launch", dockerCli)
					h.AssertNil(t, buildCacheVolume.Clear(context.TODO()))
					h.AssertNil(t, launchCacheVolume.Clear(context.TODO()))
				})

				when("daemon", func() {
					when("--run-image", func() {
						var runAfter string

						it.Before(func() {
							runAfter = registryConfig.RepoName("run-after/" + h.RandString(10))
							buildRunImage(runAfter, "contents-after-1", "contents-after-2")
						})

						it.After(func() {
							h.AssertNil(t, h.DockerRmi(dockerCli, runAfter))
						})

						it("uses provided run image", func() {
							output := pack.RunSuccessfully(
								"rebase", repoName,
								"--run-image", runAfter,
								"--no-pull",
							)

							h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
							assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
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
							h.AssertNil(t, h.DockerRmi(dockerCli, localRunImageMirror))
						})

						it("prefers the local mirror", func() {
							output := pack.RunSuccessfully("rebase", repoName, "--no-pull")

							h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s' from local config", localRunImageMirror))

							h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
							assertMockAppRunsWithOutput(t, repoName, "local-mirror-after-1", "local-mirror-after-2")
						})
					})

					when("image metadata has a mirror", func() {
						it.Before(func() {
							// clean up existing mirror first to avoid leaking images
							h.AssertNil(t, h.DockerRmi(dockerCli, runImageMirror))

							buildRunImage(runImageMirror, "mirror-after-1", "mirror-after-2")
						})

						it("selects the best mirror", func() {
							output := pack.RunSuccessfully("rebase", repoName, "--no-pull")

							h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s'", runImageMirror))

							h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
							assertMockAppRunsWithOutput(t, repoName, "mirror-after-1", "mirror-after-2")
						})
					})
				})

				when("--publish", func() {
					it.Before(func() {
						h.AssertNil(t, h.PushImage(dockerCli, repoName, registryConfig))
					})

					when("--run-image", func() {
						var runAfter string

						it.Before(func() {
							runAfter = registryConfig.RepoName("run-after/" + h.RandString(10))
							buildRunImage(runAfter, "contents-after-1", "contents-after-2")
							h.AssertNil(t, h.PushImage(dockerCli, runAfter, registryConfig))
						})

						it.After(func() {
							h.DockerRmi(dockerCli, runAfter)
						})

						it("uses provided run image", func() {
							output := pack.RunSuccessfully("rebase", repoName, "--publish", "--run-image", runAfter)

							h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
							h.AssertNil(t, h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
							assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
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

func createBuilder(t *testing.T, pack *invoke.PackInvoker, lifecycle config.LifecycleAsset, runImageMirror string) (string, error) {
	t.Log("creating builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(lifecycle.BuildpackAPIVersion())
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

	var packageImageName string
	var packageId string
	if dockerHostOS() != "windows" {
		// CREATE PACKAGE
		packageImageName = packageBuildpackAsImage(t,
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
	builderConfigFile, err := ioutil.TempFile(tmpDir, "builder.toml")
	if err != nil {
		return "", err
	}

	pack.FixtureManager().TemplateFixtureToFile(
		"builder.toml",
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

	h.AssertContains(t, output, fmt.Sprintf("Successfully created builder image '%s'", bldr))
	h.AssertNil(t, h.PushImage(dockerCli, bldr, registryConfig))

	return bldr, nil
}

func packageBuildpackAsImage(t *testing.T, pack *invoke.PackInvoker, configLocation string, lifecycle config.LifecycleAsset, buildpacks []string) string {
	tmpDir, err := ioutil.TempDir("", "package-image")
	h.AssertNil(t, err)

	outputImage := packageBuildpack(t, pack, configLocation, tmpDir, "image", lifecycle, buildpacks)

	// REGISTER CLEANUP
	key := taskKey("package-buildpack", outputImage)
	suiteManager.RegisterCleanUp("clean-"+key, func() error {
		return h.DockerRmi(dockerCli, outputImage)
	})

	return outputImage
}

func packageBuildpackAsFile(t *testing.T, pack *invoke.PackInvoker, configLocation, tmpDir string, lifecycle config.LifecycleAsset, buildpacks []string) string {
	return packageBuildpack(t, pack, configLocation, tmpDir, "file", lifecycle, buildpacks)
}

func packageBuildpack(t *testing.T, pack *invoke.PackInvoker, configLocation, tmpDir, outputFormat string, lifecycle config.LifecycleAsset, buildpacks []string) string {
	t.Helper()
	t.Log("creating package image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir(tmpDir, "create-package")
	h.AssertNil(t, err)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(lifecycle.BuildpackAPIVersion())
	t.Log("using buildpacks from: ", buildpacksDir)
	h.RecursiveCopy(t, buildpacksDir, tmpDir)

	// ARCHIVE BUILDPACKS
	for _, v := range buildpacks {
		tgz := h.CreateTGZ(t, filepath.Join(buildpacksDir, v), "./", 0755)
		err := os.Rename(tgz, filepath.Join(tmpDir, v+".tgz"))
		h.AssertNil(t, err)
	}

	packageConfig := filepath.Join(tmpDir, "package.toml")

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

	h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", outputName))

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

func assertMockAppRunsWithOutput(t *testing.T, repoName string, expectedOutputs ...string) {
	t.Helper()
	containerName := "test-" + h.RandString(10)
	runDockerImageExposePort(t, containerName, repoName)
	defer dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
	defer dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})
	launchPort := fetchHostPort(t, containerName)
	assertMockAppResponseContains(t, launchPort, 10*time.Second, expectedOutputs...)
}

func assertMockAppLogs(t *testing.T, repoName string, expectedOutputs ...string) {
	t.Helper()
	containerName := "test-" + h.RandString(10)
	ctr, err := dockerCli.ContainerCreate(context.Background(), &container.Config{
		Image: repoName,
	}, nil, nil, containerName)
	h.AssertNil(t, err)

	var b bytes.Buffer
	err = h.RunContainer(context.Background(), dockerCli, ctr.ID, &b, &b)
	h.AssertNil(t, err)

	for _, expectedOutput := range expectedOutputs {
		h.AssertContains(t, b.String(), expectedOutput)
	}
}

func assertMockAppResponseContains(t *testing.T, launchPort string, timeout time.Duration, expectedOutputs ...string) {
	t.Helper()
	resp := waitForResponse(t, launchPort, timeout)
	for _, expected := range expectedOutputs {
		h.AssertContains(t, resp, expected)
	}
}

func assertHasBase(t *testing.T, image, base string) {
	t.Helper()
	imageInspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), image)
	h.AssertNil(t, err)
	baseInspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), base)
	h.AssertNil(t, err)
	for i, layer := range baseInspect.RootFS.Layers {
		h.AssertEq(t, imageInspect.RootFS.Layers[i], layer)
	}
}

func fetchHostPort(t *testing.T, dockerID string) string {
	t.Helper()

	i, err := dockerCli.ContainerInspect(context.Background(), dockerID)
	h.AssertNil(t, err)
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

func runDockerImageExposePort(t *testing.T, containerName, repoName string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
		Image:        repoName,
		Env:          []string{"PORT=8080"},
		ExposedPorts: map[nat.Port]struct{}{"8080/tcp": {}},
		Healthcheck:  nil,
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{{}},
		},
		AutoRemove: true,
	}, nil, containerName)
	h.AssertNil(t, err)

	err = dockerCli.ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	h.AssertNil(t, err)
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
			resp, err := h.HTTPGetE("http://localhost:"+port, map[string]string{})
			if err != nil {
				break
			}
			return resp
		case <-timer.C:
			t.Fatalf("timeout waiting for response: %v", timeout)
		}
	}
}

func imageLabel(t *testing.T, dockerCli client.CommonAPIClient, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
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
