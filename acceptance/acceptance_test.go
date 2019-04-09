// +build acceptance

package acceptance

import (
	"bytes"
	"context"
	"crypto/md5"
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

	"github.com/buildpack/pack/archive"

	"github.com/buildpack/lifecycle"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	h "github.com/buildpack/pack/testhelpers"
)

var (
	packPath         string
	dockerCli        *docker.Client
	registryConfig   *h.TestRegistryConfig
	lifecycleVersion = "0.1.0"
	runImage         = "test/run"
	buildImage       = "test/build"
	runImageMirror   string
	packHome         string
	builder          string
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
	dockerCli, err = docker.New()
	h.AssertNil(t, err)
	registryConfig = h.RunRegistry(t, false)
	defer registryConfig.StopRegistry(t)
	if version, ok := os.LookupEnv("LIFECYCLE_VERSION"); ok {
		lifecycleVersion = version
	}
	runImageMirror = createStack(t, dockerCli)
	builder = createBuilder(t, runImageMirror)
	defer h.DockerRmi(dockerCli, runImage, buildImage, builder)

	spec.Run(t, "acceptance", testAcceptance, spec.Report(report.Terminal{}))
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

	when("subcommand is invalid", func() {
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

	when("pack build", func() {
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
			cacheImage, err := cache.New(repoName, dockerCli)
			h.AssertNil(t, err)
			cacheImage.Clear(context.TODO())
		})

		when("default builder is set", func() {
			it.Before(func() {
				h.Run(t, packCmd("set-default-builder", builder))
			})

			it("creates image on the daemon", func() {
				t.Log("no previous image exists")
				cmd := packCmd(
					"build", repoName,
					"-p", "testdata/mock_app/.",
				)
				output := h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				sha, err := imgSHAFromOutput(output, repoName)
				h.AssertNil(t, err)
				defer h.DockerRmi(dockerCli, sha)

				t.Log("app is runnable")
				assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

				t.Log("it uses the default run image as a base image")
				assertHasBase(t, repoName, "packs/run:"+lifecycleVersion)

				t.Log("sets the run image metadata")
				runImageLabel := imageLabel(t, dockerCli, repoName, lifecycle.MetadataLabel)
				h.AssertContains(t, runImageLabel, fmt.Sprintf(`"stack":{"runImage":{"image":"%s","mirrors":["%s"]}}}`, runImage, runImageMirror))

				t.Log("registry is empty")
				contents, err := registryConfig.RegistryCatalog()
				h.AssertNil(t, err)
				if strings.Contains(contents, repo) {
					t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
				}

				t.Log("rebuild")
				cmd = packCmd("build", repoName, "-p", "testdata/mock_app/.")
				output = h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				sha, err = imgSHAFromOutput(output, repoName)
				h.AssertNil(t, err)
				defer h.DockerRmi(dockerCli, sha)

				t.Log("app is runnable")
				assertMockAppRunsWithOutput(t, repoName, "Launch Dep Contents", "Cached Dep Contents")

				t.Log("restores the cache")
				h.AssertContainsMatch(t, output, `\[restorer] restoring cached layer 'simple/layers:cached-launch-layer'`)
				h.AssertContainsMatch(t, output, `\[analyzer] using cached launch layer 'simple/layers:cached-launch-layer'`)

				t.Log("exporter and cacher reuse unchanged layers")
				h.AssertContainsMatch(t, output, `\[exporter] reusing layer 'simple/layers:cached-launch-layer'`)
				h.AssertContainsMatch(t, output, `\[cacher] reusing layer 'simple/layers:cached-launch-layer'`)

				t.Log("rebuild with --clear-cache")
				cmd = packCmd("build", repoName, "-p", "testdata/mock_app/.", "--clear-cache")
				output = h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

				t.Log("skips restore")
				h.AssertContains(t, output, "Skipping 'restore' due to clearing cache")

				t.Log("skips analyze")
				h.AssertContains(t, output, "Skipping 'analyze' due to clearing cache")

				t.Log("exporter reuses unchanged layers")
				h.AssertContainsMatch(t, output, `\[exporter] reusing layer 'simple/layers:cached-launch-layer'`)

				t.Log("cacher adds layers")
				h.AssertContainsMatch(t, output, `\[cacher] adding layer 'simple/layers:cached-launch-layer'`)
			})

			when("--buildpack", func() {
				when("the argument is a directory or id", func() {
					it("adds the buildpacks to the builder if necessary and runs them", func() {
						skipOnWindows(t, "buildpack directories not supported on windows")
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--buildpack", filepath.Join("testdata", "mock_buildpacks", "not-in-builder-buildpack"),
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

					when("the buildpack stack doesn't match the builder", func() {
						it.Pend("errors", func() {
							skipOnWindows(t, "buildpack directories not supported on windows")
							cmd := packCmd(
								"build", repoName,
								"-p", filepath.Join("testdata", "mock_app"),
								"--buildpack", filepath.Join("testdata", "mock_buildpacks", "other-stack"),
							)
							txt, err := h.RunE(cmd)
							h.AssertNotNil(t, err)
							h.AssertContains(t, txt, "wrong stack")
						})
					})
				})
			})

			when("--env-file", func() {
				var envPath string

				it.Before(func() {
					skipOnWindows(t, "directory buildpacks are not implemented on windows")

					envfile, err := ioutil.TempFile("", "envfile")
					h.AssertNil(t, err)
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
					assertMockAppRunsWithOutput(t, repoName, "Env2 Layer Contents From Environment", "Env1 Layer Contents From File")
				})
			})

			when("--env", func() {
				it.Before(func() {
					skipOnWindows(t, "directory buildpacks are not implemented on windows")
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
						h.AssertContains(t, output, fmt.Sprintf("Pulling run image '%s'", runImageName))

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

					it.Pend("fails with a message", func() {
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--run-image", runImageName,
						)
						txt, err := h.RunE(cmd)
						h.AssertNotNil(t, err)
						h.AssertContains(t, txt, "wrong stack")
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
					imgSHA, err := imgSHAFromOutput(output, repoName)
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

					h.AssertNil(t, h.PullImageWithAuth(dockerCli, fmt.Sprintf("%s@%s", repoName, imgSHA), registryConfig.RegistryAuth()))
					defer h.DockerRmi(dockerCli, fmt.Sprintf("%s@%s", repoName, imgSHA))

					t.Log("app is runnable")
					assertMockAppRunsWithOutput(t, fmt.Sprintf("%s@%s", repoName, imgSHA), "Launch Dep Contents", "Cached Dep Contents")
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
				h.AssertContains(t, output, `Please select a default builder with:

	pack set-default-builder <builder image>`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
				h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks'`)
			})
		})
	})

	when("pack run", func() {
		it.Before(func() {
			skipOnWindows(t, "cleaning up from this test is leaving containers on windows")
		})

		when("there is a builder", func() {
			it.After(func() {
				absPath, err := filepath.Abs(filepath.Join("testdata", "mock_app"))
				h.AssertNil(t, err)
				repoName := fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(absPath)))
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
				cacheImage, err := cache.New(repoName, dockerCli)
				h.AssertNil(t, err)
				cacheImage.Clear(context.TODO())
			})

			it("starts an image", func() {
				var buf bytes.Buffer
				cmd := packCmd("run",
					"--port", "3000:8080",
					"-p", filepath.Join("testdata", "mock_app"),
					"--builder", builder,
				)
				cmd.Stdout = &buf
				cmd.Stderr = &buf
				h.AssertNil(t, cmd.Start())

				defer ctrlCProc(cmd)

				h.AssertEq(t, isCommandRunning(cmd), true)
				assertMockAppResponseContains(t, "3000", 30*time.Second, "Launch Dep Contents", "Cached Dep Contents")
			})
		})

		when("default builder is not set", func() {
			it("informs the user", func() {
				cmd := packCmd("run", "-p", filepath.Join("testdata", "mock_app"))
				output, err := h.RunE(cmd)
				h.AssertNotNil(t, err)
				h.AssertContains(t, output, `Please select a default builder with:

	pack set-default-builder <builder image>`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:bionic'`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+'cloudfoundry/cnb:cflinuxfs3'`)
				h.AssertMatch(t, output, `Heroku:\s+'heroku/buildpacks'`)
			})
		})
	})

	when("pack rebase", func() {
		var repoName, containerName, runBefore, runAfter string
		var buildRunImage func(string, string, string)

		it.Before(func() {
			containerName = "test-" + h.RandString(10)
			repoName = "some-org/" + h.RandString(10)
			runBefore = "run-before/" + h.RandString(10)
			runAfter = "run-after/" + h.RandString(10)

			buildRunImage = func(newRunImage, contents1, contents2 string) {
				h.CreateImageOnLocal(t, dockerCli, newRunImage, fmt.Sprintf(`
													FROM %s
													USER root
													RUN echo %s > /contents1.txt
													RUN echo %s > /contents2.txt
													USER pack
												`, runImage, contents1, contents2))
			}
		})

		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})

			h.DockerRmi(dockerCli, repoName)
		})

		when("run on daemon", func() {
			var origID string

			it.Before(func() {
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
				h.DockerRmi(dockerCli, origID, runBefore, runAfter)
				cacheImage, err := cache.New(repoName, dockerCli)
				h.AssertNil(t, err)
				cacheImage.Clear(context.TODO())
			})

			it("rebases", func() {
				buildRunImage(runAfter, "contents-after-1", "contents-after-2")

				cmd := packCmd("rebase", repoName, "--no-pull", "--run-image", runAfter)
				output := h.Run(t, cmd)

				h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
				assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
			})
		})

		when("--publish", func() {
			it.Before(func() {
				repoName = registryConfig.RepoName(repoName)
				runBefore = registryConfig.RepoName(runBefore)
				runAfter = registryConfig.RepoName(runAfter)

				buildRunImage(runBefore, "contents-before-1", "contents-before-2")
				h.AssertNil(t, h.PushImage(dockerCli, runBefore, registryConfig))

				cmd := packCmd("build", repoName,
					"-p", filepath.Join("testdata", "mock_app"),
					"--builder", builder,
					"--run-image", runBefore,
					"--publish")
				h.Run(t, cmd)

				h.AssertNil(t, h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
				assertMockAppRunsWithOutput(t, repoName, "contents-before-1", "contents-before-2")
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
			})

			it.After(func() {
				h.DockerRmi(dockerCli, runBefore, runAfter)
				cacheImage, err := cache.New(repoName, dockerCli)
				h.AssertNil(t, err)
				cacheImage.Clear(context.TODO())
			})

			it("rebases on the registry", func() {
				buildRunImage(runAfter, "contents-after-1", "contents-after-2")
				h.AssertNil(t, h.PushImage(dockerCli, runAfter, registryConfig))

				cmd := packCmd("rebase", repoName, "--publish", "--run-image", runAfter)
				output := h.Run(t, cmd)

				h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
				h.AssertNil(t, h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
				assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
			})
		})

		when("no run-image flag", func() {
			var origID string
			var origRunImageID string
			var builderName = "some-org/" + h.RandString(10)
			var runImage = "some-org/" + h.RandString(10)

			it.Before(func() {
				buildRunImage(runImage, "contents-before-1", "contents-before-2")
				origRunImageID = h.ImageID(t, runImage)

				h.CreateImageOnLocal(t, dockerCli, builderName, fmt.Sprintf(`
								FROM %s
								LABEL io.buildpacks.builder.metadata="{\"stack\":{\"runImage\": {\"image\": \"%s\"}}}"
								USER root
								RUN echo "[run-image]\n  image=\"%s\"" > /buildpacks/stack.toml
								USER pack
							`, builder, runImage, runImage))

				cmd := packCmd(
					"build", repoName,
					"-p", filepath.Join("testdata", "mock_app"),
					"--builder", builderName,
					"--no-pull",
				)
				h.Run(t, cmd)
				origID = h.ImageID(t, repoName)
				assertMockAppRunsWithOutput(t, repoName, "contents-before-1", "contents-before-2")
			})

			it.After(func() {
				h.DockerRmi(dockerCli, origID, builderName, origRunImageID, runImage)
				cacheImage, err := cache.New(repoName, dockerCli)
				h.AssertNil(t, err)
				cacheImage.Clear(context.TODO())
			})

			it("rebases", func() {
				buildRunImage(runImage, "contents-after-1", "contents-after-2")

				cmd := packCmd("rebase", repoName, "--no-pull")
				output := h.Run(t, cmd)

				h.AssertContains(t, output, fmt.Sprintf("Successfully rebased image '%s'", repoName))
				assertMockAppRunsWithOutput(t, repoName, "contents-after-1", "contents-after-2")
			})
		})
	})

	when("pack set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			cmd := packCmd("set-default-builder", "some/builder")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("set-default-builder command failed: %s: %s", output, err)
			}
			h.AssertEq(t, string(output), "Builder 'some/builder' is now the default builder\n")
		})
	})

	when("pack inspect-builder", func() {
		it("displays configuration for a builder (local and remote)", func() {
			configuredRunImage := "some-registry.com/some/run1"

			builderImageName := h.CreateImageOnRemote(t, dockerCli, registryConfig, "some/builder",
				fmt.Sprintf(`
										FROM scratch
										LABEL %s="{\"stack\":{\"runImage\":{\"image\":\"some/run1\",\"mirrors\":[\"gcr.io/some/run1\"]}},\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\",\"latest\":false},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\",\"latest\":true}],\"groups\":[{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\"}]},{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"}]}]}"
										LABEL io.buildpacks.stack.id=some.test.stack
									`, "io.buildpacks.builder.metadata"))

			h.CreateImageOnLocal(t, dockerCli, builderImageName,
				fmt.Sprintf(`
										FROM scratch
										LABEL %s="{\"stack\":{\"runImage\":{\"image\":\"some/run1\",\"mirrors\":[\"gcr.io/some/run2\"]}},\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\",\"latest\":false},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\",\"latest\":true}],\"groups\":[{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\"}]},{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"}]}]}"
										LABEL io.buildpacks.stack.id=some.test.stack
									`, "io.buildpacks.builder.metadata"))
			defer h.DockerRmi(dockerCli, builderImageName)

			cmd := packCmd("set-run-image-mirrors", "some/run1", "--mirror", configuredRunImage)
			output := h.Run(t, cmd)
			h.AssertEq(t, output, "Run Image 'some/run1' configured with mirror 'some-registry.com/some/run1'\n")

			cmd = packCmd("inspect-builder", builderImageName)
			output = h.Run(t, cmd)

			expected, err := ioutil.ReadFile(filepath.Join("testdata", "inspect_builder_output.txt"))
			h.AssertNil(t, err)

			h.AssertEq(t, output, fmt.Sprintf(string(expected), builderImageName))
		})
	})
}

func createBuilder(t *testing.T, runImageMirror string) string {
	skipOnWindows(t, "create builder is not implemented on windows")
	t.Log("create builder image")

	tmpDir, err := ioutil.TempDir("", "create-test-builder")
	h.AssertNil(t, err)
	defer os.RemoveAll(tmpDir)
	h.RecursiveCopy(t, filepath.Join("testdata", "mock_buildpacks"), tmpDir)
	builderConfigFile, err := os.OpenFile(filepath.Join(tmpDir, "builder.toml"), os.O_RDWR|os.O_APPEND, 0666)
	h.AssertNil(t, err)
	_, err = builderConfigFile.Write([]byte(
		fmt.Sprintf(`run-image-mirrors = ["%s"]`, runImageMirror)))
	h.AssertNil(t, err)
	builderConfigFile.Close()

	builder := "some-org/" + h.RandString(10)

	cmd := exec.Command(packPath, "create-builder", "--no-color", builder, "-b", filepath.Join(tmpDir, "builder.toml"))
	output := h.Run(t, cmd)

	h.AssertContains(t, output, fmt.Sprintf("Successfully created builder image '%s'", builder))
	return builder
}

func createStack(t *testing.T, dockerCli *docker.Client) string {
	t.Log("create stack images")
	createStackImage(t, dockerCli, "test/run", filepath.Join("testdata", "mock_stack", "run"))
	createStackImage(t, dockerCli, "test/build", filepath.Join("testdata", "mock_stack", "build"))
	runImageMirror := registryConfig.RepoName("test/run")
	err := dockerCli.ImageTag(context.Background(), runImage, runImageMirror)
	h.AssertNil(t, err)
	h.AssertNil(t, h.PushImage(dockerCli, runImageMirror, registryConfig))
	return runImageMirror
}

func createStackImage(t *testing.T, dockerCli *docker.Client, repoName string, dir string) {
	ctx := context.Background()
	buildContext, _ := archive.CreateTarReader(dir, "/", 0, 0)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
		BuildArgs:   map[string]*string{"lifecycleVersion": &lifecycleVersion},
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

func imgSHAFromOutput(txt, repoName string) (string, error) {
	for _, m := range regexp.MustCompile(`\*\*\* Image: (.+)@(.+)`).FindAllStringSubmatch(txt, -1) {
		if m[1] == repoName {
			return m[2], nil
		}
	}
	return "", fmt.Errorf("could not find Image: %s@[SHA] in output", repoName)
}

func runDockerImageExposePort(t *testing.T, containerName, repoName string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
		Image:       repoName,
		Env:         []string{"PORT=8080"},
		Healthcheck: nil,
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

func skipOnWindows(t *testing.T, message string) {
	if runtime.GOOS == "windows" {
		t.Skip(message)
	}
}

func isCommandRunning(cmd *exec.Cmd) bool {
	_, err := os.FindProcess(cmd.Process.Pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return true
}

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

func imageLabel(t *testing.T, dockerCli *docker.Client, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	label, ok := inspect.Config.Labels[labelName]
	if !ok {
		t.Errorf("expected label %s to exist", labelName)
	}
	return label
}
