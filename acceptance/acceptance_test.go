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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/Masterminds/semver"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/api"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/cache"
	"github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/style"
	h "github.com/buildpacks/pack/testhelpers"
)

const (
	envCompilePackWithVersion = "COMPILE_PACK_WITH_VERSION"

	runImage   = "pack-test/run"
	buildImage = "pack-test/build"

	defaultCompilePackVersion = "0.0.0"
	defaultPlatformAPIVersion = "0.3"
)

var (
	dockerCli      client.CommonAPIClient
	registryConfig *h.TestRegistryConfig
	suiteManager   *SuiteManager
)

type testWriter struct {
	t *testing.T
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

func TestAcceptance(t *testing.T) {
	var err error

	h.RequireDocker(t)
	rand.Seed(time.Now().UTC().UnixNano())

	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	registryConfig = h.RunRegistry(t)
	defer registryConfig.StopRegistry(t)

	testWriter := testWriter{t}
	inputPathsManager, err := NewInputPathsManager(logging.NewLogWithWriters(&testWriter, &testWriter))
	h.AssertNil(t, err)

	combos, err := getRunCombinations()
	h.AssertNil(t, err)

	// Check that the provided inputs are valid for each combo
	// If required inputs are missing, attempt to fill them in by downloading from GitHub
	for _, c := range combos {
		err := inputPathsManager.FillInRequiredPaths(c)
		h.AssertNil(t, err)
	}

	// If pack path not provided, compile pack with the version provided (or use default version)
	packPath := inputPathsManager.packPath
	if packPath == "" {
		compileVersion := os.Getenv(envCompilePackWithVersion)
		if compileVersion == "" {
			compileVersion = defaultCompilePackVersion
		}
		packPath = buildPack(t, compileVersion)
	}

	previousPackPath := inputPathsManager.previousPackPath

	// Copy previous pack fixtures directory into a temp directory
	// Copy the contents of "pack_previous_fixtures_overrides" into the temp directory
	previousPackFixturesPath := inputPathsManager.previousPackFixturesPath
	var tmpPreviousPackFixturesPath string
	if previousPackFixturesPath != "" {
		tmpPreviousPackFixturesPath, err = ioutil.TempDir("", "previous-pack-fixtures")
		h.AssertNil(t, err)
		defer os.RemoveAll(tmpPreviousPackFixturesPath)

		h.RecursiveCopy(t, previousPackFixturesPath, tmpPreviousPackFixturesPath)
		h.RecursiveCopy(t, filepath.Join("testdata", "pack_previous_fixtures_overrides"), tmpPreviousPackFixturesPath)
	}

	lifecyclePath := inputPathsManager.lifecyclePath
	lifecycleDescriptor := builder.LifecycleDescriptor{
		Info: builder.LifecycleInfo{
			Version: builder.VersionMustParse(builder.DefaultLifecycleVersion),
		},
		API: builder.LifecycleAPI{
			BuildpackVersion: api.MustParse(builder.DefaultBuildpackAPIVersion),
			PlatformVersion:  api.MustParse(defaultPlatformAPIVersion),
		},
	}
	if lifecyclePath != "" {
		lifecycleDescriptor, err = extractLifecycleDescriptor(lifecyclePath)
		if err != nil {
			t.Fatal(err)
		}
	}

	previousLifecyclePath := inputPathsManager.previousLifecyclePath
	var previousLifecycleDescriptor builder.LifecycleDescriptor
	if previousLifecyclePath != "" {
		previousLifecycleDescriptor, err = extractLifecycleDescriptor(previousLifecyclePath)
		if err != nil {
			t.Fatal(err)
		}
	}

	suiteManager = &SuiteManager{out: t.Logf}
	suite := spec.New("acceptance suite", spec.Report(report.Terminal{}))

	for _, combo := range combos {
		if combo.Pack == "current" {
			suite("p_current", func(t *testing.T, when spec.G, it spec.S) {
				testWithoutSpecificBuilderRequirement(
					t,
					when,
					it,
					currentPackFixturesDir,
					packPath,
					api.MustParse(builder.DefaultBuildpackAPIVersion),
				)
			}, spec.Report(report.Terminal{}))
			break
		}
	}

	resolvedCombos, err := resolveRunCombinations(
		combos,
		packPath,
		previousPackPath,
		tmpPreviousPackFixturesPath,
		lifecyclePath,
		lifecycleDescriptor,
		previousLifecyclePath,
		previousLifecycleDescriptor,
	)
	h.AssertNil(t, err)

	for k, combo := range resolvedCombos {
		t.Logf(`setting up run combination %s:
pack:
 |__ path: %s
 |__ fixtures: %s

create builder:
 |__ pack path: %s
 |__ pack fixtures: %s

lifecycle:
 |__ path: %s
 |__ version: %s
 |__ buildpack api: %s
 |__ platform api: %s
`,
			style.Symbol(k),
			combo.packPath,
			combo.packFixturesDir,
			combo.packCreateBuilderPath,
			combo.packCreateBuilderFixturesDir,
			combo.lifecyclePath,
			combo.lifecycleDescriptor.Info.Version,
			combo.lifecycleDescriptor.API.BuildpackVersion,
			combo.lifecycleDescriptor.API.PlatformVersion,
		)

		combo := combo

		suite(k, func(t *testing.T, when spec.G, it spec.S) {
			testAcceptance(
				t,
				when,
				it,
				combo.packFixturesDir,
				combo.packPath,
				combo.packCreateBuilderPath,
				combo.packCreateBuilderFixturesDir,
				combo.lifecyclePath,
				combo.lifecycleDescriptor,
			)
		}, spec.Report(report.Terminal{}))
	}

	suite.Run(t)

	h.AssertNil(t, suiteManager.CleanUp())
}

// These tests either (a) do not require a builder or (b) do not require a specific builder to be provided
// in order to test compatibility.
// They should only be run against the "current" (i.e., master) version of pack.
func testWithoutSpecificBuilderRequirement(
	t *testing.T,
	when spec.G,
	it spec.S,
	packFixturesDir, packPath string,
	bpVersion *api.Version,
) {
	var (
		bpDir    = buildpacksDir(*bpVersion)
		packHome string
	)

	// subjectPack creates a pack `exec.Cmd` based on the current configuration
	subjectPack := func(name string, args ...string) *exec.Cmd {
		return packCmd(packHome, packPath, name, args...)
	}

	it.Before(func() {
		var err error
		packHome, err = ioutil.TempDir("", "buildpack.pack.home.")
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(packHome))
	})

	when("invalid subcommand", func() {
		it("prints usage", func() {
			output, err := h.RunE(subjectPack("some-bad-command"))
			h.AssertNotNil(t, err)
			h.AssertContains(t, output, `unknown command "some-bad-command" for "pack"`)
			h.AssertContains(t, output, `Run 'pack --help' for usage.`)
		})
	})

	when("suggest-builders", func() {
		it("displays suggested builders", func() {
			cmd := subjectPack("suggest-builders")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("suggest-builders command failed: %s: %s", output, err)
			}
			h.AssertContains(t, string(output), "Suggested builders:")
			h.AssertContains(t, string(output), "cloudfoundry/cnb:bionic")
		})
	})

	when("suggest-stacks", func() {
		it("displays suggested stacks", func() {
			cmd := subjectPack("suggest-stacks")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("suggest-stacks command failed: %s: %s", output, err)
			}
			h.AssertContains(t, string(output), "Stacks maintained by the community:")
		})
	})

	when("set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			output := h.Run(t, subjectPack("set-default-builder", "cloudfoundry/cnb:bionic"))
			h.AssertContains(t, output, "Builder 'cloudfoundry/cnb:bionic' is now the default builder")
		})
	})

	when("package-buildpack", func() {
		var tmpDir string

		it.Before(func() {
			h.SkipIf(t,
				!packSupports(packPath, "package-buildpack"),
				"pack does not support 'package-buildpack'",
			)

			h.SkipIf(t, dockerHostOS() == "windows", "These tests are not yet compatible with Windows-based containers")

			var err error
			tmpDir, err = ioutil.TempDir("", "package-buildpack-tests")
			h.AssertNil(t, err)

			h.CopyFile(t, filepath.Join(packFixturesDir, "package.toml"), filepath.Join(tmpDir, "package.toml"))

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
			output, err := h.RunE(subjectPack("package-buildpack", packageName, "-p", absConfigPath))
			h.AssertNil(t, err)
			h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", packageName))
			return packageName
		}

		packageBuildpackRemotely := func(absConfigPath string) string {
			t.Helper()
			packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
			output, err := h.RunE(subjectPack("package-buildpack", packageName, "-p", absConfigPath, "--publish"))
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
			packageTomlData := fillTemplate(t,
				filepath.Join(packFixturesDir, "package_aggregate.toml"),
				map[string]interface{}{
					"BuildpackURI": buildpackURI,
					"PackageName":  nestedPackageName,
				},
			)
			packageTomlFile, err := ioutil.TempFile(tmpDir, "package_aggregate-*.toml")
			h.AssertNil(t, err)
			_, err = io.WriteString(packageTomlFile, packageTomlData)
			h.AssertNil(t, err)
			h.AssertNil(t, packageTomlFile.Close())

			return packageTomlFile.Name()
		}

		when("no --format is provided", func() {
			it("creates the package as image", func() {
				packageName := "test/package-" + h.RandString(10)
				output, err := h.RunE(subjectPack("package-buildpack", packageName, "-p", filepath.Join(tmpDir, "package.toml")))
				h.AssertNil(t, err)
				h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", packageName))
				defer h.DockerRmi(dockerCli, packageName)

				assertImageExistsLocally(packageName)
			})
		})

		when("--format image", func() {
			it("creates the package", func() {
				t.Log("package w/ only buildpacks")
				nestedPackageName := packageBuildpackLocally(filepath.Join(tmpDir, "package.toml"))
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
					nestedPackageName := packageBuildpackRemotely(filepath.Join(tmpDir, "package.toml"))
					defer h.DockerRmi(dockerCli, nestedPackageName)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackageName)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					output := h.Run(t, subjectPack("package-buildpack", packageName, "-p", aggregatePackageToml, "--publish"))
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
					nestedPackage := packageBuildpackLocally(filepath.Join(tmpDir, "package.toml"))
					defer h.DockerRmi(dockerCli, nestedPackage)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackage)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					h.Run(t, subjectPack("package-buildpack", packageName, "-p", aggregatePackageToml, "--no-pull"))

					_, _, err := dockerCli.ImageInspectWithRaw(context.Background(), packageName)
					h.AssertNil(t, err)

				})

				it("should not pull image from registry", func() {
					nestedPackage := packageBuildpackRemotely(filepath.Join(tmpDir, "package.toml"))
					defer h.DockerRmi(dockerCli, nestedPackage)
					aggregatePackageToml := generateAggregatePackageToml("simple-layers-parent-buildpack.tgz", nestedPackage)

					packageName := registryConfig.RepoName("test/package-" + h.RandString(10))
					defer h.DockerRmi(dockerCli, packageName)
					_, err := h.RunE(subjectPack("package-buildpack", packageName, "-p", aggregatePackageToml, "--no-pull"))
					h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", nestedPackage))
				})
			})
		})

		when("--format file", func() {
			it.Before(func() {
				h.SkipIf(t, !packSupports(packPath, "package-buildpack --format"), "format not supported")
			})

			it("creates the package", func() {
				outputFile := filepath.Join(tmpDir, "package.cnb")
				output, err := h.RunE(subjectPack("package-buildpack", outputFile, "--format", "file", "-p", filepath.Join(tmpDir, "package.toml")))
				h.AssertNil(t, err)
				h.AssertContains(t, output, fmt.Sprintf("Successfully created package '%s'", outputFile))
				h.AssertTarball(t, outputFile)
			})
		})

		when("package.toml is invalid", func() {
			it("displays an error", func() {
				h.CopyFile(t, filepath.Join(packFixturesDir, "invalid_package.toml"), filepath.Join(tmpDir, "invalid_package.toml"))

				_, err := h.RunE(subjectPack("package-buildpack", "some-package", "-p", filepath.Join(tmpDir, "invalid_package.toml")))
				h.AssertError(t, err, "reading config:")
			})
		})
	})

	when("report", func() {
		it.Before(func() {
			h.SkipIf(t, !packSupports(packPath, "report"), "pack does not support 'report' command")
		})

		when("default builder is set", func() {
			it.Before(func() {
				h.Run(t, subjectPack("set-default-builder", "cloudfoundry/cnb:bionic"))
			})

			it("outputs information", func() {
				version, err := packVersion(packPath)
				h.AssertNil(t, err)

				output := h.Run(t, subjectPack("report"))

				outputTemplate := filepath.Join(packFixturesDir, "report_output.txt")
				expectedOutput := fillTemplate(t, outputTemplate,
					map[string]interface{}{
						"DefaultBuilder": "cloudfoundry/cnb:bionic",
						"Version":        version,
						"OS":             runtime.GOOS,
						"Arch":           runtime.GOARCH,
					},
				)
				h.AssertEq(t, output, expectedOutput)
			})
		})
	})
}

func testAcceptance(
	t *testing.T,
	when spec.G,
	it spec.S,
	packFixturesDir, packPath, packCreateBuilderPath, configDir, lifecyclePath string,
	lifecycleDescriptor builder.LifecycleDescriptor,
) {
	var (
		bpDir      = buildpacksDir(*lifecycleDescriptor.API.BuildpackVersion)
		packHome   string
		packVer    string
		packSemver *semver.Version
	)

	// subjectPack creates a pack `exec.Cmd` based on the current configuration
	subjectPack := func(name string, args ...string) *exec.Cmd {
		return packCmd(packHome, packPath, name, args...)
	}

	it.Before(func() {
		var err error
		packHome, err = ioutil.TempDir("", "buildpack.pack.home.")
		h.AssertNil(t, err)
		packVer, err = packVersion(packPath)
		h.AssertNil(t, err)
		packSemver = semver.MustParse(strings.TrimPrefix(strings.Split(packVer, " ")[0], "v"))
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(packHome))
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

			it("succeeds", func() {
				builderName := createBuilder(t, runImageMirror, configDir, packCreateBuilderPath, lifecyclePath, lifecycleDescriptor)
				defer h.DockerRmi(dockerCli, builderName)

				inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), builderName)
				h.AssertNil(t, err)

				h.AssertEq(t, inspect.Os, "windows")
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

				key := taskKey("create-builder", runImageMirror, configDir, packCreateBuilderPath, lifecyclePath)
				value, err := suiteManager.RunTaskOnceString(key, func() (string, error) {
					return createBuilder(t, runImageMirror, configDir, packCreateBuilderPath, lifecyclePath, lifecycleDescriptor), nil
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
					packVer, err := packVersion(packPath)
					h.AssertNil(t, err)
					packSemver := semver.MustParse(strings.TrimPrefix(strings.Split(packVer, " ")[0], "v"))

					h.SkipIf(
						t,
						packSemver.Compare(semver.MustParse("0.9.0")) <= 0 && !packSemver.Equal(semver.MustParse("0.0.0")),
						"builder.toml validation not supported",
					)
					h.CopyFile(t, filepath.Join(packFixturesDir, "invalid_builder.toml"), filepath.Join(tmpDir, "invalid_builder.toml"))

					_, err = h.RunE(subjectPack("create-builder", "some-builder:build", "--builder-config", filepath.Join(tmpDir, "invalid_builder.toml")))
					h.AssertError(t, err, "invalid builder toml")
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

				when("default builder is set", func() {
					var creatorSupported, lifecycleImageSupported bool

					it.Before(func() {
						h.Run(t, subjectPack("set-default-builder", builderName))

						// Technically the creator is supported as of platform API version 0.3 (lifecycle version 0.7.0+) but earlier versions
						// have bugs that make using the creator problematic.
						lifecycleSupportsCreator := !lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.7.5"))
						packSupportsCreator := packSemver.GreaterThan(semver.MustParse("0.10.0")) || packSemver.Equal(semver.MustParse("0.0.0"))
						creatorSupported = lifecycleSupportsCreator && packSupportsCreator

						lifecycleImageSupported = !lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.7.5"))
					})

					it("creates a runnable, rebuildable image on daemon from app dir", func() {
						appPath := filepath.Join("testdata", "mock_app")

						var output string
						output = h.Run(t, subjectPack("build", repoName, "-p", appPath))

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
						h.Run(t, subjectPack("set-run-image-mirrors", runImage, "-m", localRunImageMirror))

						t.Log("rebuild")
						output = h.Run(t, subjectPack("build", repoName, "-p", appPath))
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
						if creatorSupported {
							h.AssertContainsMatch(t, output, `(?i)\[creator] Restoring data for "simple/layers:cached-launch-layer" from cache`)
							h.AssertContainsMatch(t, output, `(?i)\[creator] Restoring metadata for "simple/layers:cached-launch-layer" from app image`)
						} else {
							h.AssertContainsMatch(t, output, `(?i)\[restorer] Restoring data for "simple/layers:cached-launch-layer" from cache`)
							h.AssertContainsMatch(t, output, `(?i)\[analyzer] Restoring metadata for "simple/layers:cached-launch-layer" from app image`)
						}

						t.Log("exporter reuses unchanged layers")
						if creatorSupported {
							h.AssertContainsMatch(t, output, `(?i)\[creator] reusing layer 'simple/layers:cached-launch-layer'`)
						} else {
							h.AssertContainsMatch(t, output, `(?i)\[exporter] reusing layer 'simple/layers:cached-launch-layer'`)
						}

						t.Log("cacher reuses unchanged layers")
						if creatorSupported {
							h.AssertContainsMatch(t, output, `(?i)\[creator] Reusing cache layer 'simple/layers:cached-launch-layer'`)
						} else {
							h.AssertContainsMatch(t, output, `(?i)\[exporter] Reusing cache layer 'simple/layers:cached-launch-layer'`)
						}

						t.Log("rebuild with --clear-cache")
						output = h.Run(t, subjectPack("build", repoName, "-p", appPath, "--clear-cache"))
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

						t.Log("skips restore")
						if !creatorSupported {
							h.AssertContains(t, output, "Skipping 'restore' due to clearing cache")
						}

						t.Log("skips buildpack layer analysis")
						if creatorSupported {
							h.AssertContainsMatch(t, output, `(?i)\[creator] Skipping buildpack layer analysis`)
						} else {
							h.AssertContainsMatch(t, output, `(?i)\[analyzer] Skipping buildpack layer analysis`)
						}

						t.Log("exporter reuses unchanged layers")
						if creatorSupported {
							h.AssertContainsMatch(t, output, `(?i)\[creator] Reusing layer 'simple/layers:cached-launch-layer'`)
						} else {
							h.AssertContainsMatch(t, output, `(?i)\[exporter] reusing layer 'simple/layers:cached-launch-layer'`)
						}

						t.Log("cacher adds layers")
						if creatorSupported {
							h.AssertContainsMatch(t, output, `(?i)\[creator] Adding cache layer 'simple/layers:cached-launch-layer'`)
						} else {
							h.AssertContainsMatch(t, output, `(?i)\[exporter] Adding cache layer 'simple/layers:cached-launch-layer'`)
						}

						if packSupports(packPath, "inspect-image") {
							t.Log("inspect-image")
							output = h.Run(t, subjectPack("inspect-image", repoName))

							outputTemplate := filepath.Join(packFixturesDir, "inspect_image_local_output.txt")
							if _, err := os.Stat(outputTemplate); err != nil {
								t.Fatal(err.Error())
							}
							expectedOutput := fillTemplate(t, outputTemplate,
								map[string]interface{}{
									"image_name":             repoName,
									"base_image_id":          h.ImageID(t, runImageMirror),
									"base_image_top_layer":   h.TopLayerDiffID(t, runImageMirror),
									"run_image_local_mirror": localRunImageMirror,
									"run_image_mirror":       runImageMirror,
									"show_reference":         !lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.5.0")),
									"show_processes":         !lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.6.0")),
								},
							)
							h.AssertEq(t, output, expectedOutput)
						}
					})

					it("supports building app from a zip file", func() {
						appPath := filepath.Join("testdata", "mock_app.zip")
						output := h.Run(t, subjectPack("build", repoName, "-p", appPath))
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
							h.SkipIf(t, !packSupports(packPath, "build --network"), "--network flag not supported for build")

							buildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "internet-capable-buildpack"), "./", 0755)
						})

						it.After(func() {
							h.AssertNil(t, os.Remove(buildpackTgz))
							h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
						})

						when("the network mode is not provided", func() {
							it("reports that build and detect are online", func() {
								var output string
								output = h.Run(t, subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
								))

								if creatorSupported {
									h.AssertContains(t, output, "[creator] RESULT: Connected to the internet")
								} else {
									h.AssertContains(t, output, "[detector] RESULT: Connected to the internet")
									h.AssertContains(t, output, "[builder] RESULT: Connected to the internet")
								}
							})
						})

						when("the network mode is set to default", func() {
							it("reports that build and detect are online", func() {
								var output string
								output = h.Run(t, subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", buildpackTgz,
								))

								if creatorSupported {
									h.AssertContains(t, output, "[creator] RESULT: Connected to the internet")
								} else {
									h.AssertContains(t, output, "[detector] RESULT: Connected to the internet")
									h.AssertContains(t, output, "[builder] RESULT: Connected to the internet")
								}
							})
						})

						when("the network mode is set to none", func() {
							it("reports that build and detect are offline", func() {
								var output string
								output = h.Run(t, subjectPack(
									"build",
									repoName,
									"-p",
									filepath.Join("testdata", "mock_app"),
									"--buildpack",
									buildpackTgz,
									"--network",
									"none",
								))

								if creatorSupported {
									h.AssertContains(t, output, "[creator] RESULT: Disconnected from the internet")
								} else {
									h.AssertContains(t, output, "[detector] RESULT: Disconnected from the internet")
									h.AssertContains(t, output, "[builder] RESULT: Disconnected from the internet")
								}
							})
						})
					})

					when("--volume", func() {
						var buildpackTgz, tempVolume string

						it.Before(func() {
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
							h.AssertNil(t, os.Remove(buildpackTgz))
							h.AssertNil(t, h.DockerRmi(dockerCli, repoName))

							h.AssertNil(t, os.RemoveAll(tempVolume))
						})

						it("mounts the provided volume in the detect and build phases", func() {
							output := h.Run(t, subjectPack(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--buildpack", buildpackTgz,
								"--volume", fmt.Sprintf("%s:%s", tempVolume, "/my-volume-mount-target"),
							))

							if packSemver.GreaterThan(semver.MustParse("0.9.0")) || packSemver.Equal(semver.MustParse("0.0.0")) {
								h.AssertContains(t, output, "Detect: Reading file '/platform/my-volume-mount-target/some-file':")
							}
							h.AssertContains(t, output, "Build: Reading file '/platform/my-volume-mount-target/some-file':")
						})
					})

					when("--default-process", func() {
						it("sets the default process from those in the process list", func() {
							h.SkipIf(t, !packSupports(packPath, "build --default-process"), "--default-process flag is not supported")
							h.SkipIf(t,
								lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.7.0")),
								"skipping default process. Lifecycle does not support it",
							)

							h.Run(t, subjectPack(
								"build", repoName,
								"--default-process", "hello",
								"-p", filepath.Join("testdata", "mock_app"),
							))

							assertMockAppLogs(t, repoName, "hello world")
						})
					})

					when("--buildpack", func() {
						when("the argument is an ID", func() {
							it("adds the buildpacks to the builder if necessary and runs them", func() {
								output := h.Run(t, subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", "simple/layers", // Can omit version if only one
									"--buildpack", "noop.buildpack@noop.buildpack.version",
								))

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
								output := h.Run(t, subjectPack(
									"build",
									repoName,
									"-p",
									filepath.Join("testdata", "mock_app"),
									"--buildpack", localBuildpackTgz,
								))

								h.AssertContains(t, output, "Adding buildpack 'local/bp' version 'local-bp-version' to builder")
								h.AssertContains(t, output, "Build: Local Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							})
						})

						when("the argument is directory", func() {
							it("adds the buildpacks to the builder and runs it", func() {
								h.SkipIf(t, runtime.GOOS == "windows", "buildpack directories not supported on windows")

								output := h.Run(t, subjectPack(
									"build",
									repoName,
									"-p",
									filepath.Join("testdata", "mock_app"),
									"--buildpack",
									filepath.Join(bpDir, "not-in-builder-buildpack"),
								))

								h.AssertContains(t, output, "Adding buildpack 'local/bp' version 'local-bp-version' to builder")
								h.AssertContains(t, output, "Build: Local Buildpack")
								h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
							})
						})

						when("the argument is a buildpackage image", func() {
							var packageImageName string

							it.Before(func() {
								h.SkipIf(t,
									!packSupports(packPath, "package-buildpack"),
									"--buildpack does not accept buildpackage unless package-buildpack is supported",
								)

								packageImageName = packageBuildpackAsImage(t,
									packPath,
									filepath.Join(packFixturesDir, "package_for_build_cmd.toml"),
									lifecycleDescriptor,
									[]string{
										"simple-layers-parent-buildpack",
										"simple-layers-buildpack",
									},
								)
							})

							it("adds the buildpacks to the builder and runs them", func() {
								output := h.Run(t, subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", packageImageName,
								))

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
									!packSupports(packPath, "package-buildpack --format"),
									"--buildpack does not accept buildpackage file unless package-buildpack with --format is supported",
								)

								var err error
								tmpDir, err = ioutil.TempDir("", "package-file")
								h.AssertNil(t, err)

								packageFile = packageBuildpackAsFile(t,
									tmpDir,
									packPath,
									filepath.Join(packFixturesDir, "package_for_build_cmd.toml"),
									lifecycleDescriptor,
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
								output := h.Run(t, subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", packageFile,
								))

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
								txt, err := h.RunE(subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--buildpack", otherStackBuilderTgz,
								))

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
							output := h.Run(t, subjectPack(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--env-file", envPath,
							))

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
							output := h.Run(t, subjectPack(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--env", "DETECT_ENV_BUILDPACK=true",
								"--env", `ENV1_CONTENTS="Env1 Layer Contents From Command Line"`,
								"--env", "ENV2_CONTENTS",
							))

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
								output := h.Run(t, subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--run-image", runImageName,
								))
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
								txt, err := h.RunE(subjectPack(
									"build", repoName,
									"-p", filepath.Join("testdata", "mock_app"),
									"--run-image", runImageName,
								))
								h.AssertNotNil(t, err)
								h.AssertContains(t, txt, "run-image stack id 'other.stack.id' does not match builder stack 'pack.test.stack'")
							})
						})
					})

					when("--publish", func() {
						it("creates image on the registry", func() {
							output := h.Run(t, subjectPack(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--publish",
								"--network", "host",
							))
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

							if packSupports(packPath, "inspect-image") {
								t.Log("inspect-image")
								output = h.Run(t, subjectPack("inspect-image", repoName))

								outputTemplate := filepath.Join(packFixturesDir, "inspect_image_published_output.txt")
								if _, err := os.Stat(outputTemplate); err != nil {
									t.Fatal(err.Error())
								}
								expectedOutput := fillTemplate(t, outputTemplate,
									map[string]interface{}{
										"image_name":           repoName,
										"base_image_ref":       strings.Join([]string{runImageMirror, h.Digest(t, runImageMirror)}, "@"),
										"base_image_top_layer": h.TopLayerDiffID(t, runImageMirror),
										"run_image_mirror":     runImageMirror,
										"show_reference":       !lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.5.0")),
										"show_processes":       !lifecycleDescriptor.Info.Version.LessThan(semver.MustParse("0.6.0")),
									},
								)
								h.AssertEq(t, output, expectedOutput)
							}
						})

						when("builder is untrusted", func() {
							it("uses the 5 phases", func() {
								var buf bytes.Buffer
								var cmd *exec.Cmd
								cmd = subjectPack(
									"build", repoName,
									"--builder", builderName, // untrusted by default
									"-p", filepath.Join("testdata", "mock_app"),
									"--publish",
									"--network", "host",
								)

								cmd.Stdout = &buf
								cmd.Stderr = &buf

								h.AssertNil(t, cmd.Start())

								go terminateAtStep(t, cmd, &buf, "[detector]")
								err := cmd.Wait()
								h.AssertNotNil(t, err)

								h.AssertContains(t, buf.String(), "[detector]")

								if !lifecycleImageSupported {
									h.AssertContains(t, buf.String(), "Lifecycle does not have an associated lifecycle image")
								}
							})
						})

						when("builder is trusted", func() {
							it("uses the creator (when supported)", func() {
								var buf bytes.Buffer
								var cmd *exec.Cmd
								cmd = subjectPack(
									"build", repoName,
									"--builder", builderName, // untrusted by default
									"-p", filepath.Join("testdata", "mock_app"),
									"--publish",
									"--network", "host",
									"--trust-builder",
								)

								cmd.Stdout = &buf
								cmd.Stderr = &buf

								h.AssertNil(t, cmd.Start())

								if creatorSupported {
									go terminateAtStep(t, cmd, &buf, "[creator]")
									err := cmd.Wait()
									h.AssertNotNil(t, err)

									h.AssertContains(t, buf.String(), "[creator]")
								} else {
									go terminateAtStep(t, cmd, &buf, "[detector]")
									err := cmd.Wait()
									h.AssertNotNil(t, err)

									h.AssertContains(t, buf.String(), "[detector]")
								}
							})
						})
					})

					when("ctrl+c", func() {
						it("stops the execution", func() {
							var buf bytes.Buffer
							var cmd *exec.Cmd
							cmd = subjectPack("build", repoName, "-p", filepath.Join("testdata", "mock_app"))

							cmd.Stdout = &buf
							cmd.Stderr = &buf

							h.AssertNil(t, cmd.Start())

							if creatorSupported {
								go terminateAtStep(t, cmd, &buf, "[creator]")
							} else {
								go terminateAtStep(t, cmd, &buf, "[detector]")
							}

							err := cmd.Wait()
							h.AssertNotNil(t, err)
							h.AssertNotContains(t, buf.String(), "Successfully built image")
						})
					})

					when("--descriptor", func() {

						when("exclude and include", func() {
							var buildpackTgz, tempAppDir string

							it.Before(func() {
								var err error

								packVer, err := packVersion(packPath)
								h.AssertNil(t, err)
								packSemver := semver.MustParse(strings.TrimPrefix(strings.Split(packVer, " ")[0], "v"))
								supported := packSemver.GreaterThan(semver.MustParse("0.9.0")) || packSemver.Equal(semver.MustParse("0.0.0"))
								h.SkipIf(t, !supported, "pack --descriptor does NOT support 'exclude' and 'include' feature")

								buildpackTgz = h.CreateTGZ(t, filepath.Join(bpDir, "descriptor-buildpack"), "./", 0755)

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

								output := h.Run(t, subjectPack(
									"build",
									repoName,
									"-p", tempAppDir,
									"--buildpack", buildpackTgz,
									"--descriptor", excludeDescriptorPath,
								))
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

								output := h.Run(t, subjectPack(
									"build",
									repoName,
									"-p", tempAppDir,
									"--buildpack", buildpackTgz,
									"--descriptor", includeDescriptorPath,
								))
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

				when("default builder is not set", func() {
					it("informs the user", func() {
						cmd := subjectPack("build", repoName, "-p", filepath.Join("testdata", "mock_app"))
						output, err := h.RunE(cmd)
						h.AssertNotNil(t, err)
						h.AssertContains(t, output, `Please select a default builder with:`)
						h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
						h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
						h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks:18'`)
					})
				})
			})

			when("inspect-builder", func() {
				it("displays configuration for a builder (local and remote)", func() {
					configuredRunImage := "some-registry.com/pack-test/run1"
					output := h.Run(t, subjectPack("set-run-image-mirrors", "pack-test/run", "--mirror", configuredRunImage))
					h.AssertEq(t, output, "Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n")

					output = h.Run(t, subjectPack("inspect-builder", builderName))

					// Get version of pack that had created the builder
					createdByVersion, err := packVersion(packCreateBuilderPath)
					h.AssertNil(t, err)

					outputTemplate := filepath.Join(packFixturesDir, "inspect_builder_output.txt")

					// If a different version of pack had created the builder, we need a different (versioned) template for expected output
					versionedTemplate := filepath.Join(packFixturesDir, fmt.Sprintf("inspect_%s_builder_output.txt", strings.TrimPrefix(strings.Split(createdByVersion, " ")[0], "v")))
					if _, err := os.Stat(versionedTemplate); err == nil {
						outputTemplate = versionedTemplate
					} else if !os.IsNotExist(err) {
						t.Fatal(err.Error())
					}

					expectedOutput := fillTemplate(t, outputTemplate,
						map[string]interface{}{
							"builder_name":          builderName,
							"lifecycle_version":     lifecycleDescriptor.Info.Version.String(),
							"buildpack_api_version": lifecycleDescriptor.API.BuildpackVersion.String(),
							"platform_api_version":  lifecycleDescriptor.API.PlatformVersion.String(),
							"run_image_mirror":      runImageMirror,
							"pack_version":          createdByVersion,
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
					h.Run(t, subjectPack(
						"build",
						repoName,
						"-p",
						filepath.Join("testdata",
							"mock_app"),
						"--builder",
						builderName,
						"--run-image",
						runBefore,
						"--no-pull",
					))
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
							cmd := subjectPack("rebase", repoName, "--no-pull", "--run-image", runAfter)
							output := h.Run(t, cmd)

							h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
							assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
						})
					})

					when("local config has a mirror", func() {
						var localRunImageMirror string

						it.Before(func() {
							localRunImageMirror = registryConfig.RepoName("run-after/" + h.RandString(10))
							buildRunImage(localRunImageMirror, "local-mirror-after-1", "local-mirror-after-2")
							cmd := subjectPack("set-run-image-mirrors", runImage, "-m", localRunImageMirror)
							h.Run(t, cmd)
						})

						it.After(func() {
							h.AssertNil(t, h.DockerRmi(dockerCli, localRunImageMirror))
						})

						it("prefers the local mirror", func() {
							cmd := subjectPack("rebase", repoName, "--no-pull")
							output := h.Run(t, cmd)

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
							cmd := subjectPack("rebase", repoName, "--no-pull")
							output := h.Run(t, cmd)

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
							cmd := subjectPack("rebase", repoName, "--publish", "--run-image", runAfter)
							output := h.Run(t, cmd)

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

func extractLifecycleDescriptor(lcPath string) (builder.LifecycleDescriptor, error) {
	lifecycle, err := builder.NewLifecycle(blob.NewBlob(lcPath))
	if err != nil {
		return builder.LifecycleDescriptor{}, errors.Wrapf(err, "reading lifecycle from %s", lcPath)
	}

	return lifecycle.Descriptor(), nil
}

func packCmd(packHome string, packPath string, name string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{name}, args...)
	cmdArgs = append(cmdArgs, "--no-color")
	if packSupports(packPath, "--verbose") {
		cmdArgs = append(cmdArgs, "--verbose")
	}

	cmd := exec.Command(
		packPath,
		cmdArgs...,
	)

	cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+registryConfig.DockerConfigDir)
	if packHome != "" {
		cmd.Env = append(cmd.Env, "PACK_HOME="+packHome)
	}

	return cmd
}

func packVersion(packPath string) (string, error) {
	cmd := packCmd("", packPath, "version")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSpace(output)), nil
}

// packSupports returns whether or not the provided pack binary supports a
// given command string. The command string can take one of three forms:
//   - "<command>" (e.g. "create-builder")
//   - "<flag>" (e.g. "--verbose")
//   - "<command> <flag>" (e.g. "build --network")
//
// Any other form will return false.
func packSupports(packPath, command string) bool {
	parts := strings.Split(command, " ")
	var cmd, search string
	switch len(parts) {
	case 1:
		search = parts[0]
		break
	case 2:
		cmd = parts[0]
		search = parts[1]
	default:
		return false
	}

	output, err := h.RunE(exec.Command(packPath, "help", cmd))
	if err != nil {
		panic(err)
	}
	return strings.Contains(output, search)
}

func buildpacksDir(bpAPIVersion api.Version) string {
	return filepath.Join("testdata", "mock_buildpacks", bpAPIVersion.String())
}

func buildPack(t *testing.T, compileVersion string) string {
	packTmpDir, err := ioutil.TempDir("", "pack.acceptance.binary.")
	h.AssertNil(t, err)

	packPath := filepath.Join(packTmpDir, "pack")
	if runtime.GOOS == "windows" {
		packPath = packPath + ".exe"
	}

	cwd, err := os.Getwd()
	h.AssertNil(t, err)

	cmd := exec.Command("go", "build",
		"-ldflags", fmt.Sprintf("-X 'github.com/buildpacks/pack/cmd.Version=%s'", compileVersion),
		"-mod=vendor",
		"-o", packPath,
		"./cmd/pack",
	)
	if filepath.Base(cwd) == "acceptance" {
		cmd.Dir = filepath.Dir(cwd)
	}

	t.Logf("building pack: [CWD=%s] %s", cmd.Dir, cmd.Args)
	if txt, err := cmd.CombinedOutput(); err != nil {
		t.Fatal("building pack cli:\n", string(txt), err)
	}

	return packPath
}

func createBuilder(t *testing.T, runImageMirror, configDir, packPath, lifecyclePath string, lifecycleDescriptor builder.LifecycleDescriptor) string {
	t.Log("creating builder image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	h.AssertNil(t, err)
	defer os.RemoveAll(tmpDir)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(*lifecycleDescriptor.API.BuildpackVersion)
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
		h.AssertNil(t, err)
	}

	var packageImageName string
	var packageId string
	if dockerHostOS() != "windows" {
		// CREATE PACKAGE
		packageImageName = packageBuildpackAsImage(t,
			packPath,
			filepath.Join(configDir, "package.toml"),
			lifecycleDescriptor,
			[]string{"simple-layers-buildpack"},
		)

		packageId = "simple/layers"
	}

	// ADD lifecycle
	var lifecycleURI string
	var lifecycleVersion string
	if lifecyclePath != "" {
		t.Logf("adding lifecycle path '%s' to builder config", lifecyclePath)
		lifecycleURI = strings.ReplaceAll(lifecyclePath, `\`, `\\`)
	} else {
		t.Logf("adding lifecycle version '%s' to builder config", lifecycleDescriptor.Info.Version.String())
		lifecycleVersion = lifecycleDescriptor.Info.Version.String()
	}

	// RENDER builder.toml
	cfgData := fillTemplate(t, filepath.Join(configDir, "builder.toml"), map[string]interface{}{
		"package_image_name": packageImageName,
		"package_id":         packageId,
		"run_image_mirror":   runImageMirror,
		"lifecycle_uri":      lifecycleURI,
		"lifecycle_version":  lifecycleVersion,
	})

	err = ioutil.WriteFile(filepath.Join(tmpDir, "builder.toml"), []byte(cfgData), os.ModePerm)
	h.AssertNil(t, err)

	// NAME BUILDER
	bldr := registryConfig.RepoName("test/builder-" + h.RandString(10))

	// CREATE BUILDER
	cmd := exec.Command(packPath, "create-builder", "--no-color", bldr, "-b", filepath.Join(tmpDir, "builder.toml"))
	output := h.Run(t, cmd)
	h.AssertContains(t, output, fmt.Sprintf("Successfully created builder image '%s'", bldr))
	h.AssertNil(t, h.PushImage(dockerCli, bldr, registryConfig))

	return bldr
}

func packageBuildpackAsImage(t *testing.T, packPath, configPath string, lifecycleDescriptor builder.LifecycleDescriptor, buildpacks []string) string {
	tmpDir, err := ioutil.TempDir("", "package-image")
	h.AssertNil(t, err)

	outputImage := packageBuildpack(t, tmpDir, packPath, configPath, "image", lifecycleDescriptor, buildpacks)

	// REGISTER CLEANUP
	key := taskKey("package-buildpack", outputImage)
	suiteManager.RegisterCleanUp("clean-"+key, func() error {
		return h.DockerRmi(dockerCli, outputImage)
	})

	return outputImage
}

func packageBuildpackAsFile(t *testing.T, tmpDir, packPath, configPath string, lifecycleDescriptor builder.LifecycleDescriptor, buildpacks []string) string {
	return packageBuildpack(t, tmpDir, packPath, configPath, "file", lifecycleDescriptor, buildpacks)
}

func packageBuildpack(t *testing.T, tmpDir, packPath, configPath, outputFormat string, lifecycleDescriptor builder.LifecycleDescriptor, buildpacks []string) string {
	t.Helper()
	t.Log("creating package image...")

	// CREATE TEMP WORKING DIR
	tmpDir, err := ioutil.TempDir(tmpDir, "create-package")
	h.AssertNil(t, err)

	// DETERMINE TEST DATA
	buildpacksDir := buildpacksDir(*lifecycleDescriptor.API.BuildpackVersion)
	t.Log("using buildpacks from: ", buildpacksDir)
	h.RecursiveCopy(t, buildpacksDir, tmpDir)

	// ARCHIVE BUILDPACKS
	for _, v := range buildpacks {
		tgz := h.CreateTGZ(t, filepath.Join(buildpacksDir, v), "./", 0755)
		err := os.Rename(tgz, filepath.Join(tmpDir, v+".tgz"))
		h.AssertNil(t, err)
	}

	// COPY config to temp package.toml
	h.CopyFile(t, configPath, filepath.Join(tmpDir, "package.toml"))

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

	cmd := exec.Command(packPath, append([]string{
		"package-buildpack", outputName,
		"--no-color",
		"-p", filepath.Join(tmpDir, "package.toml"),
	}, additionalArgs...)...)
	cmd.Dir = tmpDir
	output := h.Run(t, cmd)
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

// FIXME : buf needs a mutex
func terminateAtStep(t *testing.T, cmd *exec.Cmd, buf *bytes.Buffer, pattern string) {
	t.Helper()
	var interruptSignal os.Signal

	if runtime.GOOS == "windows" {
		// Windows does not support os.Interrupt
		interruptSignal = os.Kill
	} else {
		interruptSignal = os.Interrupt
	}

	for {
		if strings.Contains(buf.String(), pattern) {
			h.AssertNil(t, cmd.Process.Signal(interruptSignal))
			return
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

func fillTemplate(t *testing.T, templatePath string, data map[string]interface{}) string {
	t.Helper()
	outputTemplate, err := ioutil.ReadFile(templatePath)
	h.AssertNil(t, err)

	tpl := template.Must(template.New("").Parse(string(outputTemplate)))

	var expectedOutput bytes.Buffer
	err = tpl.Execute(&expectedOutput, data)
	h.AssertNil(t, err)

	return expectedOutput.String()
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
