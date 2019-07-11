// +build acceptance

package acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	"github.com/buildpack/lifecycle/metadata"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/lifecycle"
	h "github.com/buildpack/pack/testhelpers"
)

var (
	packPath         string
	dockerCli        *client.Client
	registryConfig   *h.TestRegistryConfig
	runImage         = "pack-test/run"
	buildImage       = "pack-test/build"
	runImageMirror   string
	builder          string
	lifecycleVersion = semver.MustParse(lifecycle.DefaultLifecycleVersion)
	lifecycleV020    = semver.MustParse("0.2.0")
	lifecycleV030    = semver.MustParse("0.3.0")
)

func TestAcceptance(t *testing.T) {
	h.RequireDocker(t)
	rand.Seed(time.Now().UTC().UnixNano())

	packPath = os.Getenv("PACK_PATH")
	if packPath == "" {
		packTmpDir, err := ioutil.TempDir("", "pack.acceptance.binary.")
		if err != nil {
			t.Fatal(err)
		}
		packPath = filepath.Join(packTmpDir, "pack")
		if runtime.GOOS == "windows" {
			packPath = packPath + ".exe"
		}
		if txt, err := exec.Command("go", "build", "-o", packPath, "../cmd/pack").CombinedOutput(); err != nil {
			t.Fatal("building pack cli:\n", string(txt), err)
		}
		defer os.RemoveAll(packTmpDir)
	}
	var err error
	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)
	registryConfig = h.RunRegistry(t, false)
	defer registryConfig.StopRegistry(t)
	runImageMirror = registryConfig.RepoName(runImage)
	createStack(t, dockerCli)
	builder = createBuilder(t, runImageMirror)
	defer h.DockerRmi(dockerCli, runImage, buildImage, builder, runImageMirror)

	spec.Run(t, "pack", testAcceptance, spec.Report(report.Terminal{}))
}

func testAcceptance(t *testing.T, when spec.G, it spec.S) {
	var (
		packHome string
	)

	var packCmd = func(name string, args ...string) *exec.Cmd {
		cmdArgs := append([]string{
			name,
			"--no-color",
		}, args...)
		cmd := exec.Command(
			packPath,
			cmdArgs...,
		)
		cmd.Env = append(os.Environ(), "PACK_HOME="+packHome, "DOCKER_CONFIG="+registryConfig.DockerConfigDir)
		return cmd
	}

	it.Before(func() {
		if _, err := os.Stat(packPath); os.IsNotExist(err) {
			t.Fatal("No file found at PACK_PATH environment variable:", packPath)
		}
		var err error
		packHome, err = ioutil.TempDir("", "buildpack.pack.home.")
		h.AssertNil(t, err)
	})

	when("invalid subcommand", func() {
		it("prints usage", func() {
			cmd := packCmd("some-bad-command")
			output, _ := cmd.CombinedOutput()
			if !strings.Contains(string(output), `unknown command "some-bad-command" for "pack"`) {
				t.Fatal("Failed to print usage", string(output))
			}
			if !strings.Contains(string(output), `Run 'pack --help' for usage.`) {
				t.Fatal("Failed to print usage", string(output))
			}
		})
	})

	when("build", func() {
		var repo, repoName, containerName string

		it.Before(func() {
			repo = "some-org/" + h.RandString(10)
			repoName = registryConfig.RepoName(repo)
			containerName = "test-" + h.RandString(10)
		})

		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})
			dockerCli.ImageRemove(context.TODO(), repoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
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
			it.Before(func() {
				h.Run(t, packCmd("set-default-builder", builder))
			})

			it("creates a runnable, rebuildable image on daemon from app dir", func() {
				appPath := filepath.Join("testdata", "mock_app")
				cmd := packCmd(
					"build", repoName,
					"-p", appPath,
				)
				output := h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				imgId, err := imgIdFromOutput(output, repoName)
				if err != nil {
					t.Log(output)
					t.Fatal("Could not determine image id for built image")
				}
				defer h.DockerRmi(dockerCli, imgId)

				if lifecycleVersion.GreaterThan(lifecycleV020) || lifecycleVersion.Equal(lifecycleV020) {
					t.Log("uses a build cache volume when appropriate")
					h.AssertContains(t, output, "Using build cache volume")
				} else {
					t.Log("uses a build cache image when appropriate")
					h.AssertContains(t, output, "Using build cache image")
				}

				t.Log("app is runnable")
				assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

				t.Log("selects the best run image mirror")
				h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s'", runImageMirror))

				t.Log("it uses the run image as a base image")
				assertHasBase(t, repoName, runImage)

				t.Log("sets the run image metadata")
				runImageLabel := imageLabel(t, dockerCli, repoName, metadata.AppMetadataLabel)
				h.AssertContains(t, runImageLabel, fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImage, runImageMirror))

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
				cmd = packCmd("set-run-image-mirrors", runImage, "-m", localRunImageMirror)
				h.Run(t, cmd)

				t.Log("rebuild")
				cmd = packCmd("build", repoName, "-p", appPath)
				output = h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				imgId, err = imgIdFromOutput(output, repoName)
				if err != nil {
					t.Log(output)
					t.Fatal("Could not determine image id for built image")
				}
				defer h.DockerRmi(dockerCli, imgId)

				t.Log("local run-image mirror is selected")
				h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s' from local config", localRunImageMirror))

				t.Log("app is runnable")
				assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

				t.Log("restores the cache")
				h.AssertContainsMatch(t, output, `(?i)\[restorer] restoring cached layer 'simple/layers:cached-launch-layer'`)
				h.AssertContainsMatch(t, output, `(?i)\[analyzer] using cached launch layer 'simple/layers:cached-launch-layer'`)

				t.Log("exporter and cacher reuse unchanged layers")
				h.AssertContainsMatch(t, output, `(?i)\[exporter] reusing layer 'simple/layers:cached-launch-layer'`)
				h.AssertContainsMatch(t, output, `(?i)\[cacher] reusing layer 'simple/layers:cached-launch-layer'`)

				t.Log("rebuild with --clear-cache")
				cmd = packCmd("build", repoName, "-p", appPath, "--clear-cache")
				output = h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

				t.Log("skips restore")
				h.AssertContains(t, output, "Skipping 'restore' due to clearing cache")

				if lifecycleVersion.GreaterThan(lifecycleV030) || lifecycleVersion.Equal(lifecycleV030) {
					t.Log("skips buildpack layer analysis")
					h.AssertContainsMatch(t, output, `(?i)\[analyzer] Skipping buildpack layer analysis`)

					t.Log("exporter reuses unchanged layers")
					h.AssertContainsMatch(t, output, `(?i)\[exporter] reusing layer 'simple/layers:cached-launch-layer'`)
				}

				t.Log("cacher adds layers")
				h.AssertContainsMatch(t, output, `(?i)\[cacher] (Caching|adding) layer 'simple/layers:cached-launch-layer'`)
			})

			it("supports building app from a zip file", func() {
				appPath := filepath.Join("testdata", "mock_app.zip")
				cmd := packCmd(
					"build", repoName,
					"-p", appPath,
				)
				output := h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				imgId, err := imgIdFromOutput(output, repoName)
				if err != nil {
					t.Log(output)
					t.Fatal("Could not determine image id for built image")
				}
				defer h.DockerRmi(dockerCli, imgId)
			})

			when("--buildpack", func() {
				when("the argument is a tgz or id", func() {
					var notBuilderTgz string

					it.Before(func() {
						notBuilderTgz = h.CreateTgz(t, filepath.Join("testdata", "mock_buildpacks", "not-in-builder-buildpack"), "./", 0766)
					})

					it.After(func() {
						h.AssertNil(t, os.Remove(notBuilderTgz))
					})

					it("adds the buildpacks to the builder if necessary and runs them", func() {
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--buildpack", notBuilderTgz,
							"--buildpack", "simple/layers@simple-layers-version",
							"--buildpack", "noop.buildpack",
						)
						output := h.Run(t, cmd)
						h.AssertContains(t, output, "NOOP Buildpack")
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
						t.Log("app is runnable")
						assertMockAppRunsWithOutput(t, repoName,
							"Local Buildpack Dep Contents",
							"Launch Dep Contents",
							"Cached Dep Contents",
						)
					})

				})

				when("the argument is directory", func() {
					it("adds the buildpacks to the builder if necessary and runs them", func() {
						h.SkipIf(t, runtime.GOOS == "windows", "buildpack directories not supported on windows")

						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--buildpack", filepath.Join("testdata", "mock_buildpacks", "not-in-builder-buildpack"),
						)
						output := h.Run(t, cmd)
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
						t.Log("app is runnable")
						assertMockAppRunsWithOutput(t, repoName, "Local Buildpack Dep Contents")
					})
				})

				when("the buildpack stack doesn't match the builder", func() {
					var otherStackBuilderTgz string

					it.Before(func() {
						otherStackBuilderTgz = h.CreateTgz(t, filepath.Join("testdata", "mock_buildpacks", "other-stack-buildpack"), "./", 0766)
					})

					it.After(func() {
						h.AssertNil(t, os.Remove(otherStackBuilderTgz))
					})

					it("errors", func() {
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--buildpack", otherStackBuilderTgz,
						)
						txt, err := h.RunE(cmd)
						h.AssertNotNil(t, err)
						h.AssertContains(t, txt, "buildpack 'other/stack/bp' version 'other-stack-version' does not support stack 'pack.test.stack'")
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
					cmd := packCmd(
						"build", repoName,
						"-p", filepath.Join("testdata", "mock_app"),
						"--env-file", envPath,
					)
					output := h.Run(t, cmd)
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
					cmd := packCmd(
						"build", repoName,
						"-p", filepath.Join("testdata", "mock_app"),
						"--env", "DETECT_ENV_BUILDPACK=true",
						"--env", `ENV1_CONTENTS="Env1 Layer Contents From Command Line"`,
						"--env", "ENV2_CONTENTS",
					)
					output := h.Run(t, cmd)
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
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--run-image", runImageName,
						)

						output := h.Run(t, cmd)
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
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--run-image", runImageName,
						)
						txt, err := h.RunE(cmd)
						h.AssertNotNil(t, err)
						h.AssertContains(t, txt, "run-image stack id 'other.stack.id' does not match builder stack 'pack.test.stack'")
					})
				})
			})

			when("--publish", func() {
				it("creates image on the registry", func() {
					runPackBuild := func() string {
						t.Helper()
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--publish",
						)
						return h.Run(t, cmd)
					}
					output := runPackBuild()
					h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
					imgDigest, err := imgDigestFromOutput(output, repoName)
					if err != nil {
						t.Log(output)
						t.Fatal("Could not determine sha for built image")
					}

					t.Log("Checking that registry has contents")
					contents, err := registryConfig.RegistryCatalog()
					h.AssertNil(t, err)
					if !strings.Contains(contents, repo) {
						t.Fatalf("Expected to see image %s in %s", repo, contents)
					}

					h.AssertNil(t, h.PullImageWithAuth(dockerCli, fmt.Sprintf("%s@%s", repoName, imgDigest), registryConfig.RegistryAuth()))
					defer h.DockerRmi(dockerCli, fmt.Sprintf("%s@%s", repoName, imgDigest))

					t.Log("app is runnable")
					assertMockAppRunsWithOutput(t, fmt.Sprintf("%s@%s", repoName, imgDigest), "Launch Dep Contents", "Cached Dep Contents")
				})
			})

			when("ctrl+c", func() {
				it("stops the execution", func() {
					var buf bytes.Buffer
					cmd := packCmd("build", repoName, "-p", filepath.Join("testdata", "mock_app"))
					cmd.Stdout = &buf
					cmd.Stderr = &buf

					h.AssertNil(t, cmd.Start())

					go terminateAtStep(t, cmd, &buf, "[detector]")

					err := cmd.Wait()
					h.AssertNotNil(t, err)
					h.AssertNotContains(t, buf.String(), "Successfully built image")
				})
			})
		})

		when("default builder is not set", func() {
			it("informs the user", func() {
				cmd := packCmd("build", repoName, "-p", filepath.Join("testdata", "mock_app"))
				output, err := h.RunE(cmd)
				h.AssertNotNil(t, err)
				h.AssertContains(t, output, `Please select a default builder with:`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
				h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks:18'`)
			})
		})
	})

	when("run", func() {
		it.Before(func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Skipping because windows fails to clean up properly")
		})

		when("there is a builder", func() {
			var (
				listeningPort string
				err           error
			)

			it.Before(func() {
				listeningPort, err = h.GetFreePort()
				h.AssertNil(t, err)
			})

			it.After(func() {
				absPath, err := filepath.Abs(filepath.Join("testdata", "mock_app"))
				h.AssertNil(t, err)

				sum := sha256.Sum256([]byte(absPath))
				repoName := fmt.Sprintf("pack.local/run/%x", sum[:8])
				ref, err := name.ParseReference(repoName, name.WeakValidation)
				h.AssertNil(t, err)

				h.DockerRmi(dockerCli, repoName)

				cache.NewImageCache(ref, dockerCli).Clear(context.TODO())
				cache.NewVolumeCache(ref, "build", dockerCli).Clear(context.TODO())
				cache.NewVolumeCache(ref, "launch", dockerCli).Clear(context.TODO())
			})

			it("starts an image", func() {
				var buf bytes.Buffer
				cmd := packCmd("run",
					"--port", listeningPort+":8080",
					"-p", filepath.Join("testdata", "mock_app"),
					"--builder", builder,
				)
				cmd.Stdout = &buf
				cmd.Stderr = &buf
				h.AssertNil(t, cmd.Start())

				defer ctrlCProc(cmd)

				h.AssertEq(t, isCommandRunning(cmd), true)
				assertMockAppResponseContains(t, listeningPort, 30*time.Second, "Launch Dep Contents", "Cached Dep Contents")
			})
		})

		when("default builder is not set", func() {
			it("informs the user", func() {
				cmd := packCmd("run", "-p", filepath.Join("testdata", "mock_app"))
				output, err := h.RunE(cmd)
				h.AssertNotNil(t, err)
				h.AssertContains(t, output, `Please select a default builder with:`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
				h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks:18'`)
			})
		})
	})

	when("rebase", func() {
		var repoName, runBefore, origID string
		var buildRunImage func(string, string, string)

		it.Before(func() {
			repoName = registryConfig.RepoName("some-org/" + h.RandString(10))
			runBefore = registryConfig.RepoName("run-before/" + h.RandString(10))

			buildRunImage = func(newRunImage, contents1, contents2 string) {
				h.CreateImageOnLocal(t, dockerCli, newRunImage, fmt.Sprintf(`
													FROM %s
													USER root
													RUN echo %s > /contents1.txt
													RUN echo %s > /contents2.txt
													USER pack
												`, runImage, contents1, contents2))
			}
			buildRunImage(runBefore, "contents-before-1", "contents-before-2")

			cmd := packCmd(
				"build", repoName,
				"-p", filepath.Join("testdata", "mock_app"),
				"--builder", builder,
				"--run-image", runBefore,
				"--no-pull",
			)
			h.Run(t, cmd)
			origID = h.ImageID(t, repoName)
			assertMockAppRunsWithOutput(t, repoName, "contents-before-1", "contents-before-2")
		})

		it.After(func() {
			h.AssertNil(t, h.DockerRmi(dockerCli, origID, repoName, runBefore))
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
					cmd := packCmd("rebase", repoName, "--no-pull", "--run-image", runAfter)
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
					cmd := packCmd("set-run-image-mirrors", runImage, "-m", localRunImageMirror)
					h.Run(t, cmd)
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, localRunImageMirror))
				})

				it("prefers the local mirror", func() {
					cmd := packCmd("rebase", repoName, "--no-pull")
					output := h.Run(t, cmd)

					h.AssertContains(t, output, fmt.Sprintf("Selected run image mirror '%s' from local config", localRunImageMirror))

					h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
					assertMockAppRunsWithOutput(t, repoName, "local-mirror-after-1", "local-mirror-after-2")
				})
			})

			when("image metadata has a mirror", func() {
				it.Before(func() {
					//clean up existing mirror first to avoid leaking images
					h.AssertNil(t, h.DockerRmi(dockerCli, runImageMirror))

					buildRunImage(runImageMirror, "mirror-after-1", "mirror-after-2")
				})

				it("selects the best mirror", func() {
					cmd := packCmd("rebase", repoName, "--no-pull")
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
					cmd := packCmd("rebase", repoName, "--publish", "--run-image", runAfter)
					output := h.Run(t, cmd)

					h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
					h.AssertNil(t, h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
					assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
				})
			})
		})
	})

	when("suggest-builders", func() {
		it("displays suggested builders", func() {
			cmd := packCmd("suggest-builders")
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
			cmd := packCmd("suggest-stacks")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("suggest-stacks command failed: %s: %s", output, err)
			}
			h.AssertContains(t, string(output), "Stacks maintained by the Cloud Native Buildpacks project:")
			h.AssertContains(t, string(output), "Stacks maintained by the community:")
		})
	})

	when("set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			cmd := packCmd("set-default-builder", "cloudfoundry/cnb:bionic")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("set-default-builder command failed: %s: %s", output, err)
			}
			h.AssertContains(t, string(output), "Builder 'cloudfoundry/cnb:bionic' is now the default builder")
		})
	})

	when("inspect-builder", func() {
		it("displays configuration for a builder (local and remote)", func() {
			configuredRunImage := "some-registry.com/pack-test/run1"
			cmd := packCmd("set-run-image-mirrors", "pack-test/run", "--mirror", configuredRunImage)
			output := h.Run(t, cmd)
			h.AssertEq(t, output, "Run Image 'pack-test/run' configured with mirror 'some-registry.com/pack-test/run1'\n")

			cmd = packCmd("inspect-builder", builder)
			output = h.Run(t, cmd)
			expected, err := ioutil.ReadFile(filepath.Join("testdata", "inspect_builder_output.txt"))
			h.AssertNil(t, err)
			h.AssertEq(t, output, fmt.Sprintf(string(expected), builder, lifecycleVersion, runImageMirror, lifecycleVersion, runImageMirror))
		})
	})
}

func createBuilder(t *testing.T, runImageMirror string) string {
	t.Log("create builder image")

	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	h.AssertNil(t, err)
	defer os.RemoveAll(tmpDir)

	h.RecursiveCopy(t, filepath.Join("testdata", "mock_buildpacks"), tmpDir)

	buildpacks := []string{
		"noop-buildpack",
		"not-in-builder-buildpack",
		"other-stack-buildpack",
		"read-env-buildpack",
		"simple-layers-buildpack",
	}

	for _, v := range buildpacks {
		tgz := h.CreateTgz(t, filepath.Join("testdata", "mock_buildpacks", v), "./", 0766)
		err := os.Rename(tgz, filepath.Join(tmpDir, v+".tgz"))
		h.AssertNil(t, err)
	}

	builderConfigFile, err := os.OpenFile(filepath.Join(tmpDir, "builder.toml"), os.O_RDWR|os.O_APPEND, 0666)
	h.AssertNil(t, err)

	_, err = builderConfigFile.Write([]byte(fmt.Sprintf("run-image-mirrors = [\"%s\"]\n", runImageMirror)))
	h.AssertNil(t, err)

	_, err = builderConfigFile.Write([]byte("[lifecycle]\n"))
	h.AssertNil(t, err)
	if lifecyclePath, ok := os.LookupEnv("LIFECYCLE_PATH"); ok {
		lifecycleVersion = semver.MustParse("0.0.0")
		if !filepath.IsAbs(lifecyclePath) {
			t.Fatal("LIFECYCLE_PATH must be an absolute path")
		}
		t.Logf("Adding lifecycle path '%s' to builder config", lifecyclePath)
		_, err = builderConfigFile.Write([]byte(fmt.Sprintf("uri = \"%s\"\n", strings.ReplaceAll(lifecyclePath, `\`, `\\`))))
		h.AssertNil(t, err)
	}
	if lcver, ok := os.LookupEnv("LIFECYCLE_VERSION"); ok {
		lifecycleVersion = semver.MustParse(lcver)
		t.Logf("Adding lifecycle version '%s' to builder config", lifecycleVersion)
		_, err = builderConfigFile.Write([]byte(fmt.Sprintf("version = \"%s\"\n", lifecycleVersion.String())))
		h.AssertNil(t, err)
	}

	builderConfigFile.Close()

	builder := registryConfig.RepoName("some-org/" + h.RandString(10))

	t.Logf("Creating builder. Lifecycle version '%s' will be used.", lifecycleVersion)
	cmd := exec.Command(packPath, "create-builder", "--no-color", builder, "-b", filepath.Join(tmpDir, "builder.toml"))
	output := h.Run(t, cmd)
	h.AssertContains(t, output, fmt.Sprintf("Successfully created builder image '%s'", builder))
	h.AssertNil(t, h.PushImage(dockerCli, builder, registryConfig))

	return builder
}

func createStack(t *testing.T, dockerCli *client.Client) {
	t.Log("create stack images")
	createStackImage(t, dockerCli, runImage, filepath.Join("testdata", "mock_stack"))
	h.AssertNil(t, dockerCli.ImageTag(context.Background(), runImage, buildImage))
	h.AssertNil(t, dockerCli.ImageTag(context.Background(), runImage, runImageMirror))
	h.AssertNil(t, h.PushImage(dockerCli, runImageMirror, registryConfig))
}

func createStackImage(t *testing.T, dockerCli *client.Client, repoName string, dir string) {
	ctx := context.Background()
	buildContext := archive.ReadDirAsTar(dir, "/", 0, 0, -1)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	})
	h.AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
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

func assertMockAppResponseContains(t *testing.T, launchPort string, timeout time.Duration, expectedOutputs ...string) {
	t.Helper()
	resp := waitForResponse(t, launchPort, timeout)
	for _, expected := range expectedOutputs {
		h.AssertContains(t, resp, expected)
	}
}

func assertHasBase(t *testing.T, image, base string) {
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

func imgDigestFromOutput(txt, repoName string) (string, error) {
	if lifecycleVersion.LessThan(lifecycleV030) {
		for _, m := range regexp.MustCompile(`\*\*\* Image: (.+)@(.+)`).FindAllStringSubmatch(txt, -1) {
			if m[1] == repoName || m[1] == repoName+":latest" {
				return m[2], nil
			}
		}
	}

	for _, m := range regexp.MustCompile(`\*\*\* Digest: (.+)`).FindAllStringSubmatch(txt, -1) {
		return m[1], nil
	}

	return "", errors.New("could not find digest in output")
}

func imgIdFromOutput(txt, repoName string) (string, error) {
	if lifecycleVersion.LessThan(lifecycleV030) {
		return imgDigestFromOutput(txt, repoName)
	}

	for _, m := range regexp.MustCompile(`\*\*\* Image ID: (.+)`).FindAllStringSubmatch(txt, -1) {
		return m[1], nil
	}

	return "", errors.New("could not find image ID in output")
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
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := h.HttpGetE("http://localhost:"+port, map[string]string{})
			if err != nil {
				break
			}
			return resp
		case <-timer.C:
			t.Fatalf("timeout waiting for response: %v", timeout)
		}
	}
}

func ctrlCProc(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return fmt.Errorf("invalid pid: %#v", cmd)
	}
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return err
	}
	_, err := cmd.Process.Wait()
	return err
}

func isCommandRunning(cmd *exec.Cmd) bool {
	_, err := os.FindProcess(cmd.Process.Pid)
	if err != nil {
		return false
	}
	return true
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

func imageLabel(t *testing.T, dockerCli *client.Client, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	label, ok := inspect.Config.Labels[labelName]
	if !ok {
		t.Errorf("expected label %s to exist", labelName)
	}
	return label
}
