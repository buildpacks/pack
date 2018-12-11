package acceptance

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
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

	"github.com/BurntSushi/toml"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/docker"
	h "github.com/buildpack/pack/testhelpers"
)

var pack string
var dockerCli *docker.Client
var registryPort string
var packTag string

func TestPack(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	pack = os.Getenv("PACK_PATH")
	if pack == "" {
		packTmpDir, err := ioutil.TempDir("", "pack.acceptance.binary.")
		if err != nil {
			t.Fatal(err)
		}
		pack = filepath.Join(packTmpDir, "pack")
		if runtime.GOOS == "windows" {
			pack = pack + ".exe"
		}
		if txt, err := exec.Command("go", "build", "-o", pack, "../cmd/pack").CombinedOutput(); err != nil {
			t.Fatal("building pack cli:\n", string(txt), err)
		}
		defer os.RemoveAll(packTmpDir)
	}

	var err error
	dockerCli, err = docker.New()
	h.AssertNil(t, err)
	registryPort = h.RunRegistry(t, true)
	defer h.StopRegistry(t)
	defer h.CleanDefaultImages(t, registryPort)

	spec.Run(t, "pack", testPack, spec.Report(report.Terminal{}))
}

func testPack(t *testing.T, when spec.G, it spec.S) {
	var packHome string

	it.Before(func() {
		if _, err := os.Stat(pack); os.IsNotExist(err) {
			t.Fatal("No file found at PACK_PATH environment variable:", pack)
		}

		var err error
		packHome, err = ioutil.TempDir("", "buildpack.pack.home.")
		h.AssertNil(t, err)
		h.ConfigurePackHome(t, packHome, registryPort)
	})

	it.After(func() {
		os.RemoveAll(packHome)
	})

	when("subcommand is invalid", func() {
		it("prints usage", func() {
			cmd := exec.Command(pack, "some-bad-command")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, _ := cmd.CombinedOutput()
			if !strings.Contains(string(output), `unknown command "some-bad-command" for "pack"`) {
				t.Fatal("Failed to print usage", string(output))
			}
			if !strings.Contains(string(output), `Run 'pack --help' for usage.`) {
				t.Fatal("Failed to print usage", string(output))
			}
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack build", func() {
		var sourceCodePath, repo, repoName, containerName string

		it.Before(func() {
			repo = "some-org/" + h.RandString(10)
			repoName = "localhost:" + registryPort + "/" + repo
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
			if sourceCodePath != "" {
				os.RemoveAll(sourceCodePath)
			}
		})

		when("'--publish' flag is not specified'", func() {
			it("builds and exports an image", func() {
				cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath)
				cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
				h.Run(t, cmd)

				runDockerImageExposePort(t, containerName, repoName)
				launchPort := fetchHostPort(t, containerName)

				waitForPort(t, launchPort, 10*time.Second)
				h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Buildpacks Worked! - 1000:1000")

				t.Log("Checking that registry is empty")
				contents := h.HttpGet(t, fmt.Sprintf("http://localhost:%s/v2/_catalog", registryPort))
				if strings.Contains(string(contents), repo) {
					t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
				}
			})
		}, spec.Parallel(), spec.Report(report.Terminal{}))

		when("'--buildpack' flag is specified", func() {
			javaBpId := "io.buildpacks.samples.java"
			it.Before(func() {
				var err error
				sourceCodePath, err = ioutil.TempDir("", "pack.build.maven_app.")
				if err != nil {
					t.Fatal(err)
				}
				h.AssertNil(t, copyDirectory("testdata/maven_app/.", sourceCodePath))
			})

			// Skip this test for now. The container run at the very end runs java -jar target/testdata-sample-app-1.0-SNAPSHOT.jar
			// instead of java -jar target/testdata-sample-app-1.0-SNAPSHOT-jar-with-dependencies.jar, which ends
			// up exiting immediately
			it.Pend("assumes latest if no version is provided", func() {
				cmd := exec.Command(pack, "build", repoName, "--buildpack", javaBpId, "-p", sourceCodePath)
				cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
				buildOutput := h.Run(t, cmd)

				h.AssertEq(t, strings.Contains(buildOutput, "DETECTING WITH MANUALLY-PROVIDED GROUP:"), true)
				if strings.Contains(buildOutput, "Node.js Buildpack") {
					t.Fatalf("should have skipped Node.js buildpack because --buildpack flag was provided")
				}
				latestInfo := fmt.Sprintf(`No version for '%s' buildpack provided, will use '%s@latest'`, javaBpId, javaBpId)
				if !strings.Contains(buildOutput, latestInfo) {
					t.Fatalf(`expected build output to contain "%s", got "%s"`, latestInfo, buildOutput)
				}
				h.AssertContains(t, buildOutput, "Sample Java Buildpack: pass")

				runDockerImageExposePort(t, containerName, repoName)
				launchPort := fetchHostPort(t, containerName)

				waitForPort(t, launchPort, 10*time.Second)
				h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Maven buildpack worked!")
			})
		})

		when("'--publish' flag is specified", func() {
			it("builds and exports an image", func() {
				runPackBuild := func() string {
					t.Helper()
					cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath, "--publish")
					cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
					return h.Run(t, cmd)
				}
				output := runPackBuild()
				imgSHA, err := imgSHAFromOutput(output, repoName)
				if err != nil {
					fmt.Println(output)
					t.Fatal("Could not determine sha for built image")
				}

				t.Log("Checking that registry has contents")
				contents := h.HttpGet(t, fmt.Sprintf("http://localhost:%s/v2/_catalog", registryPort))
				if !strings.Contains(string(contents), repo) {
					t.Fatalf("Expected to see image %s in %s", repo, contents)
				}

				h.AssertNil(t, h.PullImage(dockerCli, fmt.Sprintf("%s@%s", repoName, imgSHA)))
				defer h.DockerRmi(dockerCli, fmt.Sprintf("%s@%s", repoName, imgSHA))

				ctrID := runDockerImageExposePort(t, containerName, fmt.Sprintf("%s@%s", repoName, imgSHA))
				defer dockerCli.ContainerRemove(context.TODO(), ctrID, dockertypes.ContainerRemoveOptions{Force: true})

				launchPort := fetchHostPort(t, containerName)

				waitForPort(t, launchPort, 10*time.Second)
				h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Buildpacks Worked! - 1000:1000")

				t.Log("uses the cache on subsequent run")
				output = runPackBuild()

				regex := regexp.MustCompile(`moved \d+ packages`)
				if !regex.MatchString(output) {
					t.Fatalf("Build failed to use cache: %s", output)
				}
			})
		}, spec.Parallel(), spec.Report(report.Terminal{}))
	}, spec.Parallel(), spec.Report(report.Terminal{}))

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
			repoName := fmt.Sprintf("pack.local/run/%x", md5.Sum([]byte(sourceCodePath)))
			h.AssertNil(t, h.DockerRmi(dockerCli, repoName))

			if sourceCodePath != "" {
				os.RemoveAll(sourceCodePath)
			}
		})

		it("starts an image", func() {
			var buf bytes.Buffer
			cmd := exec.Command(pack, "run", "--port", "3000", "-p", sourceCodePath)
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			cmd.Dir = sourceCodePath
			h.AssertNil(t, cmd.Start())

			defer ctrlCProc(cmd)

			h.Eventually(t, func() bool {
				return strings.Contains(buf.String(), "Example app listening on port 3000!")
			}, time.Second, 2*time.Minute)

			txt := h.HttpGet(t, "http://localhost:3000")
			h.AssertEq(t, txt, "Buildpacks Worked! - 1000:1000")
		})

	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack rebase", func() {
		var repoName, containerName, runBefore, runAfter string
		var buildAndSetRunImage func(runImage, contents1, contents2 string)
		var rootContents1 func() string
		it.Before(func() {
			containerName = "test-" + h.RandString(10)
			repoName = "some-org/" + h.RandString(10)
			runBefore = "run-before/" + h.RandString(10)
			runAfter = "run-after/" + h.RandString(10)

			buildAndSetRunImage = func(runImage, contents1, contents2 string) {
				h.CreateImageOnLocal(t, dockerCli, runImage, fmt.Sprintf(`
					FROM %s
					USER root
					RUN echo %s > /contents1.txt
					RUN echo %s > /contents2.txt
					USER pack
				`, h.DefaultRunImage(t, registryPort), contents1, contents2))

				cmd := exec.Command(
					pack, "update-stack",
					"io.buildpacks.stacks.bionic",
					"--build-image", h.DefaultBuildImage(t, registryPort),
					"--run-image", runImage)
				cmd.Env = []string{"PACK_HOME=" + packHome}
				h.Run(t, cmd)
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

			h.AssertNil(t, h.DockerRmi(dockerCli, repoName, runBefore, runAfter))
		})

		when("run on daemon", func() {
			var origID string
			it.Before(func() {
				buildAndSetRunImage(runBefore, "contents-before-1", "contents-before-2")

				cmd := exec.Command(pack, "build", repoName, "-p", "testdata/node_app/", "--no-pull")
				cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
				h.Run(t, cmd)
				origID = h.ImageID(t, repoName)
			})
			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, origID))
			})

			it("rebases", func() {
				buildAndSetRunImage(runBefore, "contents-before-1", "contents-before-2")

				h.AssertEq(t, rootContents1(), "contents-before-1\n")

				buildAndSetRunImage(runAfter, "contents-after-1", "contents-after-2")

				cmd := exec.Command(pack, "rebase", repoName, "--no-pull")
				cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
				h.Run(t, cmd)

				h.AssertEq(t, rootContents1(), "contents-after-1\n")
			})
		})

		when("run on registry", func() {
			it.Before(func() {
				repoName = "localhost:" + registryPort + "/" + repoName
				runBefore = "localhost:" + registryPort + "/" + runBefore
				runAfter = "localhost:" + registryPort + "/" + runAfter

				buildAndSetRunImage(runBefore, "contents-before-1", "contents-before-2")
				h.AssertNil(t, pushImage(dockerCli, runBefore))

				cmd := exec.Command(pack, "build", repoName, "-p", "testdata/node_app/", "--publish")
				cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
				h.Run(t, cmd)

				h.AssertNil(t, h.PullImage(dockerCli, repoName))
				h.AssertEq(t, rootContents1(), "contents-before-1\n")
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
			})

			it("rebases", func() {
				buildAndSetRunImage(runAfter, "contents-after-1", "contents-after-2")
				h.AssertNil(t, pushImage(dockerCli, runAfter))

				cmd := exec.Command(pack, "rebase", repoName, "--publish")
				cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
				h.Run(t, cmd)
				h.AssertNil(t, h.PullImage(dockerCli, repoName))

				h.AssertEq(t, rootContents1(), "contents-after-1\n")
			})
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack create-builder", func() {
		var (
			builderRepoName string
			containerName   string
			repoName        string
		)

		it.Before(func() {
			skipOnWindows(t, "create builder is not implemented on windows")
			builderRepoName = "some-org/" + h.RandString(10)
			repoName = "some-org/" + h.RandString(10)
			containerName = "test-" + h.RandString(10)
		})

		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ImageRemove(context.TODO(), builderRepoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
		})

		it("builds and exports an image", func() {
			builderTOML := filepath.Join("testdata", "mock_buildpacks", "builder.toml")
			sourceCodePath := filepath.Join("testdata", "mock_app")

			t.Log("create builder image")
			cmd := exec.Command(
				pack, "create-builder",
				builderRepoName,
				"-b", builderTOML,
			)
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			h.Run(t, cmd)

			t.Log("build uses order defined in builder.toml")
			cmd = exec.Command(
				pack, "build", repoName,
				"--builder", builderRepoName,
				"--no-pull",
				"--path", sourceCodePath,
			)
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			buildOutput, err := cmd.CombinedOutput()
			h.AssertNil(t, err)
			defer func(origID string) { h.AssertNil(t, h.DockerRmi(dockerCli, origID)) }(h.ImageID(t, repoName))
			if !strings.Contains(string(buildOutput), "First Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "First Mock Buildpack: pass", buildOutput)
			}
			if !strings.Contains(string(buildOutput), "Second Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Second Mock Buildpack: pass", buildOutput)
			}
			if !strings.Contains(string(buildOutput), "Third Mock Buildpack: pass") {
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
			cmd = exec.Command(
				pack, "build", repoName,
				"--builder", builderRepoName,
				"--no-pull",
				"--buildpack", "mock.bp.first",
				"--buildpack", "mock.bp.third@0.0.3-mock",
				"--path", sourceCodePath,
			)
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			buildOutput, err = cmd.CombinedOutput()
			h.AssertNil(t, err)
			defer func(origID string) { h.AssertNil(t, h.DockerRmi(dockerCli, origID)) }(h.ImageID(t, repoName))
			latestInfo := `No version for 'mock.bp.first' buildpack provided, will use 'mock.bp.first@latest'`
			if !strings.Contains(string(buildOutput), latestInfo) {
				t.Fatalf(`expected build output to contain "%s", got "%s"`, latestInfo, buildOutput)
			}
			if !strings.Contains(string(buildOutput), "Latest First Mock Buildpack: pass") {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, "Latest First Mock Buildpack: pass", buildOutput)
			}
			if !strings.Contains(string(buildOutput), "Third Mock Buildpack: pass") {
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
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack add-stack", func() {
		it("adds a custom stack to ~/.pack/config.toml", func() {
			cmd := exec.Command(pack, "add-stack", "my.custom.stack", "--run-image", "my-org/run", "--build-image", "my-org/build")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output := h.Run(t, cmd)

			h.AssertEq(t, string(output), "my.custom.stack successfully added\n")

			var config struct {
				Stacks []struct {
					ID          string   `toml:"id"`
					BuildImages []string `toml:"build-images"`
					RunImages   []string `toml:"run-images"`
				} `toml:"stacks"`
			}
			_, err := toml.DecodeFile(filepath.Join(packHome, "config.toml"), &config)
			h.AssertNil(t, err)

			stack := config.Stacks[len(config.Stacks)-1]
			h.AssertEq(t, stack.ID, "my.custom.stack")
			h.AssertEq(t, stack.BuildImages, []string{"my-org/build"})
			h.AssertEq(t, stack.RunImages, []string{"my-org/run"})
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack update-stack", func() {
		type config struct {
			Stacks []struct {
				ID          string   `toml:"id"`
				BuildImages []string `toml:"build-images"`
				RunImages   []string `toml:"run-images"`
			} `toml:"stacks"`
		}

		it.Before(func() {
			cmd := exec.Command(pack, "add-stack", "my.custom.stack", "--run-image", "my-org/run", "--build-image", "my-org/build")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("add-stack command failed: %s: %s", output, err)
			}
		})

		it("updates an existing custom stack in ~/.pack/config.toml", func() {
			cmd := exec.Command(pack, "update-stack", "my.custom.stack", "--run-image", "my-org/run-2", "--run-image", "my-org/run-3", "--build-image", "my-org/build-2", "--build-image", "my-org/build-3")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("update-stack command failed: %s: %s", output, err)
			}
			h.AssertEq(t, string(output), "my.custom.stack successfully updated\n")

			var config config
			_, err = toml.DecodeFile(filepath.Join(packHome, "config.toml"), &config)
			h.AssertNil(t, err)

			stack := config.Stacks[len(config.Stacks)-1]
			h.AssertEq(t, stack.ID, "my.custom.stack")
			h.AssertEq(t, stack.BuildImages, []string{"my-org/build-2", "my-org/build-3"})
			h.AssertEq(t, stack.RunImages, []string{"my-org/run-2", "my-org/run-3"})
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack set-default-stack", func() {
		type config struct {
			DefaultStackID string `toml:"default-stack-id"`
		}

		it.Before(func() {
			cmd := exec.Command(pack, "add-stack", "my.custom.stack", "--run-image", "my-org/run", "--build-image", "my-org/build")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("add-stack command failed: %s: %s", output, err)
			}
		})

		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			cmd := exec.Command(
				pack,
				"set-default-stack",
				"my.custom.stack",
			)
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("set-default-stack command failed: %s: %s", output, err)
			}
			h.AssertEq(t, string(output), "my.custom.stack is now the default stack\n")

			var config config
			_, err = toml.DecodeFile(filepath.Join(packHome, "config.toml"), &config)
			h.AssertNil(t, err)
			h.AssertEq(t, config.DefaultStackID, "my.custom.stack")
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack delete-stack", func() {
		type config struct {
			Stacks []struct {
				ID          string   `toml:"id"`
				BuildImages []string `toml:"build-images"`
				RunImages   []string `toml:"run-images"`
			} `toml:"stacks"`
		}
		containsStack := func(c config, stackID string) bool {
			for _, s := range c.Stacks {
				if s.ID == stackID {
					return true
				}
			}
			return false
		}

		it.Before(func() {
			cmd := exec.Command(pack, "add-stack", "my.custom.stack", "--run-image", "my-org/run", "--build-image", "my-org/build")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("add-stack command failed: %s: %s", output, err)
			}
		})

		it("deletes a custom stack from ~/.pack/config.toml", func() {
			var config config
			_, err := toml.DecodeFile(filepath.Join(packHome, "config.toml"), &config)
			h.AssertNil(t, err)
			numStacks := len(config.Stacks)
			h.AssertEq(t, containsStack(config, "my.custom.stack"), true)

			cmd := exec.Command(pack, "delete-stack", "my.custom.stack")
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("add-stack command failed: %s: %s", output, err)
			}
			h.AssertEq(t, string(output), "my.custom.stack has been successfully deleted\n")

			_, err = toml.DecodeFile(filepath.Join(packHome, "config.toml"), &config)
			h.AssertNil(t, err)
			h.AssertEq(t, len(config.Stacks), numStacks-1)
			h.AssertEq(t, containsStack(config, "my.custom.stack"), false)
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("pack set-default-builder", func() {
		type config struct {
			DefaultBuilder string `toml:"default-builder"`
		}

		it("sets the default-stack-id in ~/.pack/config.toml", func() {
			cmd := exec.Command(
				pack,
				"set-default-builder",
				"some/builder",
			)
			cmd.Env = append(os.Environ(), "PACK_HOME="+packHome)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("set-default-builder command failed: %s: %s", output, err)
			}
			h.AssertEq(t, string(output), "Successfully set 'some/builder' as default builder.\n")

			var config config
			_, err = toml.DecodeFile(filepath.Join(packHome, "config.toml"), &config)
			h.AssertNil(t, err)
			h.AssertEq(t, config.DefaultBuilder, "some/builder")
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))
}

// TODO: fetchHostPort, proxyDockerHostPort, and runRegistry are duplicated
// here and in build_test.go. Find a common spot for them.
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

func pushImage(d *docker.Client, ref string) error {
	rc, err := d.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{
		RegistryAuth: base64.StdEncoding.EncodeToString([]byte("{}")),
	})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func waitForPort(t *testing.T, port string, duration time.Duration) {
	h.Eventually(t, func() bool {
		_, err := h.HttpGetE("http://localhost:" + port)
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

func killDockerByRepoName(t *testing.T, repoName string) {
	t.Helper()
	containers, err := dockerCli.ContainerList(context.Background(), dockertypes.ContainerListOptions{})
	h.AssertNil(t, err)

	for _, ctr := range containers {
		if ctr.Image == repoName {
			h.AssertNil(t, dockerCli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{Force: true}))
		}
	}
}

func skipOnWindows(t *testing.T, message string) {
	if runtime.GOOS == "windows" {
		t.Skip(message)
	}
}
