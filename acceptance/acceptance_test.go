// +build acceptance

package acceptance

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
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

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	h "github.com/buildpack/pack/testhelpers"
)

var packPath string
var dockerCli *docker.Client
var registryConfig *h.TestRegistryConfig

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
	registryConfig = h.RunRegistry(t, true)
	defer registryConfig.StopRegistry(t)
	defer h.CleanDefaultImages(t, registryConfig.RunRegistryPort)

	spec.Run(t, "acceptance", testAcceptance, spec.Report(report.Terminal{}))
}

func testAcceptance(t *testing.T, when spec.G, it spec.S) {
	var packHome string

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
		h.ConfigurePackHome(t, packHome, registryConfig.RunRegistryPort)
	})

	it.After(func() {
		os.RemoveAll(packHome)
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
		var sourceCodePath, repo, repoName, containerName string

		it.Before(func() {
			repo = "some-org/" + h.RandString(10)
			repoName = registryConfig.RepoName(repo)
			containerName = "test-" + h.RandString(10)

			var err error
			sourceCodePath, err = ioutil.TempDir("", "pack.build.node_app.")
			if err != nil {
				t.Fatal(err)
			}
			h.AssertNil(t, copyDirectory("testdata/node_app/.", sourceCodePath))
		})

		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})
			dockerCli.ImageRemove(context.TODO(), repoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
			cacheImage, err := cache.New(repoName, dockerCli)
			h.AssertNil(t, err)
			cacheImage.Clear(context.TODO())
			if sourceCodePath != "" {
				os.RemoveAll(sourceCodePath)
			}
		})

		when("default builder is set", func() {
			it.Before(func() {
				h.Run(t, packCmd("set-default-builder", h.DefaultBuilderImage(t, registryConfig.RunRegistryPort)))
			})

			it("creates image on the daemon", func() {
				t.Log("no previous image exists")
				cmd := packCmd("build", repoName, "-p", "testdata/node_app/.")
				output := h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				sha, err := imgSHAFromOutput(output, repoName)
				h.AssertNil(t, err)
				defer h.DockerRmi(dockerCli, sha)

				t.Log("app is runnable")
				assertNodeAppRuns(t, repoName)

				t.Log("it uses the default run image as a base image")
				assertHasBase(t, repoName, h.DefaultRunImage(t, registryConfig.RunRegistryPort))

				t.Log("sets the run image label")
				runImageLabel := imageLabel(t, dockerCli, repoName, "io.buildpacks.run-image")
				h.AssertEq(t, runImageLabel, h.DefaultRunImage(t, registryConfig.RunRegistryPort))

				t.Log("registry is empty")
				contents, err := registryConfig.RegistryCatalog()
				h.AssertNil(t, err)
				if strings.Contains(contents, repo) {
					t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
				}

				t.Log("rebuild")
				cmd = packCmd("build", repoName, "-p", "testdata/node_app/.")
				output = h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
				sha, err = imgSHAFromOutput(output, repoName)
				h.AssertNil(t, err)
				defer h.DockerRmi(dockerCli, sha)

				t.Log("app is runnable")
				assertNodeAppRuns(t, repoName)

				t.Log("restores the cache")
				h.AssertContainsMatch(t, output, `\[restorer] \S+ \S+ restoring cached layer 'io.buildpacks.samples.nodejs:nodejs'`)
				h.AssertContainsMatch(t, output, `\[analyzer] \S+ \S+ using cached launch layer 'io.buildpacks.samples.nodejs:nodejs'`)

				t.Log("exporter and cacher reuse unchanged layers")
				h.AssertContainsMatch(t, output, `\[exporter] \S+ \S+ reusing layer 'io.buildpacks.samples.nodejs:nodejs'`)
				h.AssertContainsMatch(t, output, `\[cacher] \S+ \S+ reusing layer 'io.buildpacks.samples.nodejs:nodejs'`)

				t.Log("rebuild with --clear-cache")
				cmd = packCmd("build", repoName, "-p", "testdata/node_app/.", "--clear-cache")
				output = h.Run(t, cmd)
				h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

				t.Log("doesn't restore the cache")
				h.AssertContains(t, output, "nothing to restore")

				t.Log("exporter reuses unchanged layers")
				h.AssertContainsMatch(t, output, `\[exporter] \S+ \S+ reusing layer 'io.buildpacks.samples.nodejs:nodejs'`)

				t.Log("cacher adds layers")
				h.AssertContainsMatch(t, output, `\[cacher] \S+ \S+ adding layer 'io.buildpacks.samples.nodejs:nodejs'`)
			})

			when("--buildpack", func() {
				when("the argument is a directory", func() {
					it("adds the buildpack to the builder and runs it", func() {
						skipOnWindows(t, "buildpack directories not supported on windows")
						cmd := packCmd(
							"build", repoName,
							"-p", filepath.Join("testdata", "mock_app"),
							"--buildpack", filepath.Join("testdata", "mock_buildpacks", "first"),
							"--buildpack", filepath.Join("testdata", "mock_buildpacks", "second"),
						)
						output := h.Run(t, cmd)
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
						t.Log("it calls /bin/detect on the buildpacks")
						if !strings.Contains(output, "First Mock Buildpack: pass") {
							t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "First Mock Buildpack: pass", output)
						}
						if !strings.Contains(output, "Second Mock Buildpack: pass") {
							t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Second Mock Buildpack: pass", output)
						}
						t.Log("it calls /bin/build on the buildpacks")
						if !strings.Contains(output, "---> First mock buildpack") {
							t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "---> First mock buildpack", output)
						}
						if !strings.Contains(output, "---> Second mock buildpack") {
							t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "---> Second mock buildpack", output)
						}
						t.Log("run app container")
						runOutput := runDockerImageWithOutput(t, containerName, repoName)
						if !strings.Contains(runOutput, "First Dep Contents") {
							t.Fatalf(`Expected output to contain "First Dep Contents", got "%s"`, runOutput)
						}
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
				//--buildpack flag with id arg is tested in create-builder test
			})

			when("--env-file", func() {
				var envPath string

				it.Before(func() {
					skipOnWindows(t, "directory buildpacks are not implemented on windows")

					envfile, err := ioutil.TempFile("", "envfile")
					h.AssertNil(t, err)
					err = os.Setenv("VAR3", "value from env")
					h.AssertNil(t, err)
					envfile.WriteString(`
			VAR1=value1
			VAR2=value2
			VAR3
			`)
					envPath = envfile.Name()
				})

				it.After(func() {
					h.AssertNil(t, os.Unsetenv("VAR3"))
					h.AssertNil(t, os.RemoveAll(envPath))
				})

				it("provides the env vars to the build and detect steps", func() {
					cmd := packCmd(
						"build", repoName,
						"-p", filepath.Join("testdata", "mock_app"),
						"--env-file", envPath,
						"--buildpack", filepath.Join("testdata", "mock_buildpacks", "printenv"),
					)
					output := h.Run(t, cmd)
					h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))
					h.AssertContains(t, output, "DETECT: VAR1 is value1;")
					h.AssertContains(t, output, "DETECT: VAR2 is value2;")
					h.AssertContains(t, output, "DETECT: VAR3 is value from env;")
					h.AssertContains(t, output, "BUILD: VAR1 is value1;")
					h.AssertContains(t, output, "BUILD: VAR2 is value2;")
					h.AssertContains(t, output, "BUILD: VAR3 is value from env;")
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
												`, h.DefaultRunImage(t, registryConfig.RunRegistryPort)))
					})

					it.After(func() {
						h.DockerRmi(dockerCli, runImageName)
					})

					it("uses the run image as the base image", func() {
						cmd := packCmd(
							"build", repoName,
							"-p", "testdata/node_app/.",
							"--run-image", runImageName,
						)

						output := h.Run(t, cmd)
						h.AssertContains(t, output, fmt.Sprintf("Successfully built image '%s'", repoName))

						t.Log("app is runnable")
						assertNodeAppRuns(t, repoName)

						t.Log("pulls the run image")
						h.AssertContains(t, output, fmt.Sprintf("Pulling run image '%s'", runImageName))

						t.Log("uses the run image as the base image")
						assertHasBase(t, repoName, runImageName)

						t.Log("doesn't set the run-image label")
						assertImageLabelAbsent(t, dockerCli, repoName, "io.buildpacks.run-image")
					})
				})

				when("the run image has the wrong stack ID", func() {
					it.Before(func() {
						runImageName = h.CreateImageOnRemote(t, dockerCli, registryConfig, "custom-run-image"+h.RandString(10), fmt.Sprintf(`
													FROM %s
													LABEL io.buildpacks.stack.id=other.stack.id
													USER pack
												`, h.DefaultRunImage(t, registryConfig.RunRegistryPort)))

					})

					it.After(func() {
						h.DockerRmi(dockerCli, runImageName)
					})

					it.Pend("fails with a message", func() {
						cmd := packCmd(
							"build", repoName,
							"-p", "testdata/node_app/.",
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
						cmd := packCmd("build", repoName, "-p", sourceCodePath, "--publish")
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
					assertNodeAppRuns(t, fmt.Sprintf("%s@%s", repoName, imgSHA))
				})
			})

			when("ctrl+c", func() {
				it("stops the execution", func() {
					var buf bytes.Buffer
					cmd := packCmd("build", repoName, "-p", sourceCodePath)
					cmd.Stdout = &buf
					cmd.Stderr = &buf

					h.AssertNil(t, cmd.Start())

					go terminateAtStep(t, cmd, &buf, "[detector]")

					err := cmd.Wait()
					h.AssertNotNil(t, err)
					h.AssertNotContains(t, buf.String(), "Successfully built image")

					time.Sleep(5 * time.Second)
				})
			})

			//--no-pull flag is tested in create-builder test
		})

		when("default builder is not set", func() {
			it("informs the user", func() {
				cmd := packCmd("build", repoName, "-p", "testdata/node_app/.")
				output, err := h.RunE(cmd)
				h.AssertNotNil(t, err)
				h.AssertContains(t, output, `Please select a default builder with:

	pack set-default-builder [builder image]`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+cloudfoundry/cnb`)
				h.AssertMatch(t, output, `Heroku:\s+heroku/buildpacks`)
				h.AssertMatch(t, output, `Samples:\s+packs/samples`)
			})
		})

		//--builder flag is tested in create-builder test
	})

	when("pack run", func() {
		var sourceCodePath string

		it.Before(func() {
			skipOnWindows(t, "cleaning up from this test is leaving containers on windows")
			var err error
			sourceCodePath, err = ioutil.TempDir("", "pack.build.node_app.")
			if err != nil {
				t.Fatal(err)
			}
			h.AssertNil(t, copyDirectory("testdata/node_app/.", sourceCodePath))
		})

		it.After(func() {
			os.RemoveAll(sourceCodePath)
		})

		when("default builder is set", func() {
			it.Before(func() {
				h.Run(t, packCmd("set-default-builder", h.DefaultBuilderImage(t, registryConfig.RunRegistryPort)))
			})

			it.After(func() {
				repoName := fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(sourceCodePath)))
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
				cacheImage, err := cache.New(repoName, dockerCli)
				h.AssertNil(t, err)
				cacheImage.Clear(context.TODO())
				if sourceCodePath != "" {
					os.RemoveAll(sourceCodePath)
				}
			})

			it("starts an image", func() {
				var buf bytes.Buffer
				cmd := packCmd("run", "--port", "3000", "-p", sourceCodePath)
				cmd.Stdout = &buf
				cmd.Stderr = &buf
				cmd.Dir = sourceCodePath
				h.AssertNil(t, cmd.Start())

				defer ctrlCProc(cmd)

				h.Eventually(t, func() bool {
					if !isCommandRunning(cmd) {
						t.Fatalf("Command exited unexpectedly: \n %s", buf.String())
					}

					return strings.Contains(buf.String(), "Example app listening on port 3000!")
				}, time.Second, 2*time.Minute)

				txt := h.HttpGet(t, "http://localhost:3000")
				h.AssertEq(t, txt, "Buildpacks Worked! - 1000:1000")
			})
		})

		when("default builder is not set", func() {
			it("informs the user", func() {
				cmd := packCmd("run", "-p", "testdata/node_app/.")
				output, err := h.RunE(cmd)
				h.AssertNotNil(t, err)
				h.AssertContains(t, output, `Please select a default builder with:

	pack set-default-builder [builder image]`)
				h.AssertMatch(t, output, `Cloud Foundry:\s+cloudfoundry/cnb`)
				h.AssertMatch(t, output, `Heroku:\s+heroku/buildpacks`)
				h.AssertMatch(t, output, `Samples:\s+packs/samples`)
			})
		})
	})

	when("pack rebase", func() {
		var repoName, containerName, runBefore, runAfter string
		var buildRunImage func(string, string, string)
		var rootContents1 func() string

		it.Before(func() {
			containerName = "test-" + h.RandString(10)
			repoName = "some-org/" + h.RandString(10)
			runBefore = "run-before/" + h.RandString(10)
			runAfter = "run-after/" + h.RandString(10)

			buildRunImage = func(runImage, contents1, contents2 string) {
				h.CreateImageOnLocal(t, dockerCli, runImage, fmt.Sprintf(`
													FROM %s
													USER root
													RUN echo %s > /contents1.txt
													RUN echo %s > /contents2.txt
													USER pack
												`, h.DefaultRunImage(t, registryConfig.RunRegistryPort), contents1, contents2))
			}
			rootContents1 = func() string {
				t.Helper()
				runDockerImageExposePort(t, containerName, repoName)
				defer func() {
					dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
					dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})
				}()

				launchPort := fetchHostPort(t, containerName)
				waitForPort(t, launchPort, 10*time.Second)
				h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Buildpacks Worked! - 1000:1000")
				txt := h.HttpGet(t, "http://localhost:"+launchPort+"/rootcontents1")
				h.AssertNil(t, dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL"))
				return txt
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
					"-p", "testdata/node_app/",
					"--builder", h.DefaultBuilderImage(t, registryConfig.RunRegistryPort),
					"--run-image", runBefore,
					"--no-pull",
				)
				h.Run(t, cmd)
				origID = h.ImageID(t, repoName)
				h.AssertEq(t, rootContents1(), "contents-before-1\n")
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
				h.AssertEq(t, rootContents1(), "contents-after-1\n")
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
					"-p", "testdata/node_app/",
					"--builder", h.DefaultBuilderImage(t, registryConfig.RunRegistryPort),
					"--run-image", runBefore,
					"--publish")
				h.Run(t, cmd)

				h.AssertNil(t, h.PullImageWithAuth(dockerCli, repoName, registryConfig.RegistryAuth()))
				h.AssertEq(t, rootContents1(), "contents-before-1\n")
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
				h.AssertEq(t, rootContents1(), "contents-after-1\n")
			})
		})

		//todo do we need this test and if so how do we cleanup
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
								LABEL %s="{\"runImage\": {\"image\": \"%s\"}}"
							`, h.DefaultBuilderImage(t, registryConfig.RunRegistryPort), pack.BuilderMetadataLabel, runImage))

				cmd := packCmd(
					"build", repoName,
					"-p", "testdata/node_app/",
					"--builder", builderName,
					"--no-pull",
				)
				h.Run(t, cmd)
				origID = h.ImageID(t, repoName)
				h.AssertEq(t, rootContents1(), "contents-before-1\n")
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
				h.AssertEq(t, rootContents1(), "contents-after-1\n")
			})
		})
	})

	when("pack create-builder", func() {
		var (
			builderRepoName string
			containerName   string
			repoName        string
			builderTOML     string
			tmpDir          string
		)

		it.Before(func() {
			skipOnWindows(t, "create builder is not implemented on windows")
			builderRepoName = "some-org/" + h.RandString(10)
			repoName = "some-org/" + h.RandString(10)
			containerName = "test-" + h.RandString(10)
			if h.PackTag() != h.DefaultTag {
				var err error
				tmpDir, err = ioutil.TempDir("", "create-builder-toml-with-tags")
				h.AssertNil(t, err)
				h.RecursiveCopy(t, filepath.Join("testdata", "mock_buildpacks"), tmpDir)
				builderTOML = filepath.Join(tmpDir, "builder.toml")
				builderTOMLContents, err := ioutil.ReadFile(builderTOML)
				h.AssertNil(t, err)
				newBuilderTOML := strings.Replace(
					string(builderTOMLContents),
					"packs/build:"+h.DefaultTag,
					h.DefaultBuildImage(t, registryConfig.RunRegistryPort),
					-1,
				)
				newBuilderTOML = strings.Replace(
					string(newBuilderTOML),
					"packs/run:"+h.DefaultTag,
					h.DefaultRunImage(t, registryConfig.RunRegistryPort),
					-1,
				)
				err = ioutil.WriteFile(builderTOML, []byte(newBuilderTOML), 0777)
				h.AssertNil(t, err)
			} else {
				builderTOML = filepath.Join("testdata", "mock_buildpacks", "builder.toml")
			}
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ImageRemove(context.TODO(), builderRepoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
			dockerCli.ImageRemove(context.TODO(), repoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
			cacheImage, err := cache.New(repoName, dockerCli)
			h.AssertNil(t, err)
			cacheImage.Clear(context.TODO())
		})

		it("builds and exports an image", func() {
			sourceCodePath := filepath.Join("testdata", "mock_app")

			t.Log("create builder image")
			cmd := packCmd("create-builder", builderRepoName, "-b", builderTOML)
			output := h.Run(t, cmd)
			h.AssertContains(t, output, fmt.Sprintf("Successfully created builder image '%s'", builderRepoName))

			t.Log("build uses order defined in builder.toml")
			cmd = packCmd("build", repoName, "--builder", builderRepoName, "--path", sourceCodePath, "--no-pull")
			buildOutput := h.Run(t, cmd)
			defer func(origID string) { h.AssertNil(t, h.DockerRmi(dockerCli, origID)) }(h.ImageID(t, repoName))
			if !strings.Contains(buildOutput, "First Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "First Mock Buildpack: pass", buildOutput)
			}
			if !strings.Contains(buildOutput, "Second Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Second Mock Buildpack: pass", buildOutput)
			}
			if !strings.Contains(buildOutput, "Third Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Third Mock Buildpack: pass", buildOutput)
			}

			t.Log("run app container")
			runOutput := runDockerImageWithOutput(t, containerName, repoName)
			if !strings.Contains(runOutput, "First Dep Contents") {
				t.Fatalf(`Expected output to contain "First Dep Contents", got "%s"`, runOutput)
			}
			if !strings.Contains(runOutput, "Second Dep Contents") {
				t.Fatalf(`Expected output to contain "First Dep Contents", got "%s"`, runOutput)
			}
			if !strings.Contains(runOutput, "Third Dep Contents") {
				t.Fatalf(`Expected output to contain "Third Dep Contents", got "%s"`, runOutput)
			}

			t.Log("build with multiple --buildpack flags")
			cmd = packCmd(
				"build", repoName,
				"--builder", builderRepoName,
				"--buildpack", "mock.bp.first",
				"--buildpack", "mock.bp.third@0.0.3-mock",
				"--path", sourceCodePath,
				"--no-pull",
			)
			buildOutput = h.Run(t, cmd)
			defer func(origID string) { h.AssertNil(t, h.DockerRmi(dockerCli, origID)) }(h.ImageID(t, repoName))
			latestInfo := "No version for 'mock.bp.first' buildpack provided, will use 'mock.bp.first@latest'"
			if !strings.Contains(buildOutput, latestInfo) {
				t.Fatalf(`expected build output to contain "%s", got "%s"`, latestInfo, buildOutput)
			}
			if !strings.Contains(buildOutput, "Latest First Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Latest First Mock Buildpack: pass", buildOutput)
			}
			if !strings.Contains(buildOutput, "Third Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Third Mock Buildpack: pass", buildOutput)
			}

			t.Log("run app container")
			runOutput = runDockerImageWithOutput(t, containerName, repoName)
			if !strings.Contains(runOutput, "Latest First Dep Contents") {
				t.Fatalf(`Expected output to contain "First Dep Contents", got "%s"`, runOutput)
			}
			if strings.Contains(runOutput, "Second Dep Contents") {
				t.Fatalf(`Should not have run second buildpack, got "%s"`, runOutput)
			}
			if !strings.Contains(runOutput, "Third Dep Contents") {
				t.Fatalf(`Expected output to contain "Third Dep Contents", got "%s"`, runOutput)
			}
		})
	})

	when("pack set-default-builder", func() {
		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			cmd := packCmd("set-default-builder", "some/builder")
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
										LABEL %s="{\"runImage\":{\"image\":\"some/run1\",\"mirrors\":[\"gcr.io/some/run1\"]},\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\",\"latest\":false},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\",\"latest\":true}],\"groups\":[{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\"}]},{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"}]}]}"
										LABEL io.buildpacks.stack.id=some.test.stack
									`, pack.BuilderMetadataLabel))

			h.CreateImageOnLocal(t, dockerCli, builderImageName,
				fmt.Sprintf(`
										FROM scratch
										LABEL %s="{\"runImage\":{\"image\":\"some/run1\",\"mirrors\":[\"gcr.io/some/run2\"]},\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\",\"latest\":false},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\",\"latest\":true}],\"groups\":[{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"},{\"id\":\"test.bp.two\",\"version\":\"0.0.2\"}]},{\"buildpacks\":[{\"id\":\"test.bp.one\",\"version\":\"0.0.1\"}]}]}"
										LABEL io.buildpacks.stack.id=some.test.stack
									`, pack.BuilderMetadataLabel))
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

func assertNodeAppRuns(t *testing.T, repoName string) {
	t.Helper()
	containerName := "test-" + h.RandString(10)
	runDockerImageExposePort(t, containerName, repoName)
	defer dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
	defer dockerCli.ContainerRemove(context.TODO(), containerName, dockertypes.ContainerRemoveOptions{Force: true})
	launchPort := fetchHostPort(t, containerName)
	waitForPort(t, launchPort, 10*time.Second)
	h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Buildpacks Worked! - 1000:1000")
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

func copyDirectory(srcDir, destDir string) error {
	destExists, _ := fileExists(destDir)
	if !destExists {
		return errors.New("destination dir must exist")
	}
	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		src := filepath.Join(srcDir, f.Name())
		dest := filepath.Join(destDir, f.Name())
		if m := f.Mode(); m&os.ModeSymlink != 0 {
			target, err := os.Readlink(src)
			if err != nil {
				return fmt.Errorf("Error while reading symlink '%s': %v", src, err)
			}
			if err := os.Symlink(target, dest); err != nil {
				return fmt.Errorf("Error while creating '%s' as symlink to '%s': %v", dest, target, err)
			}
		} else if f.IsDir() {
			err = os.MkdirAll(dest, f.Mode())
			if err != nil {
				return err
			}
			if err := copyDirectory(src, dest); err != nil {
				return err
			}
		} else {
			rc, err := os.Open(src)
			if err != nil {
				return err
			}
			err = writeToFile(rc, dest, f.Mode())
			if err != nil {
				rc.Close()
				return err
			}
			rc.Close()
		}
	}
	return nil
}

func fileExists(file string) (bool, error) {
	_, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
func writeToFile(source io.Reader, destFile string, mode os.FileMode) error {
	err := os.MkdirAll(filepath.Dir(destFile), 0755)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(destFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(fh, source)
	if err != nil {
		return err
	}
	return nil
}

func runDockerImageExposePort(t *testing.T, containerName, repoName string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
		Image: repoName,
		Env:   []string{"PORT=8080"},
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

func runDockerImageWithOutput(t *testing.T, containerName, repoName string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
		Image: repoName,
	}, &container.HostConfig{}, nil, containerName)
	h.AssertNil(t, err)
	defer dockerCli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	var buf bytes.Buffer
	err = dockerCli.RunContainer(ctx, ctr.ID, &buf, &buf)
	h.AssertNil(t, err)

	return buf.String()
}

func waitForPort(t *testing.T, port string, duration time.Duration) {
	h.Eventually(t, func() bool {
		_, err := h.HttpGetE("http://localhost:"+port, map[string]string{})
		return err == nil
	}, 500*time.Millisecond, duration)
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

func assertImageLabelAbsent(t *testing.T, dockerCli *docker.Client, repoName, labelName string) {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	val, ok := inspect.Config.Labels[labelName]
	if ok {
		t.Errorf("expected label %s not to exist but was %s", labelName, val)
	}
}
