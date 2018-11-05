package acceptance

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/pack/docker"
	h "github.com/buildpack/pack/testhelpers"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var pack string
var dockerCli *docker.Client

func TestPack(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	pack = os.Getenv("PACK_PATH")
	if pack == "" {
		packTmpDir, err := ioutil.TempDir("", "pack.acceptance.binary.")
		if err != nil {
			panic(err)
		}
		if txt, err := exec.Command("go", "build", "-o", filepath.Join(packTmpDir, "pack"), "../cmd/pack").CombinedOutput(); err != nil {
			fmt.Println(string(txt))
			panic(err)
		}
		pack = filepath.Join(packTmpDir, "pack")
		defer os.RemoveAll(packTmpDir)
	}

	var err error
	dockerCli, err = docker.New()
	h.AssertNil(t, err)
	h.AssertNil(t, dockerCli.PullImage("sclevine/test"))
	h.AssertNil(t, dockerCli.PullImage("packs/samples"))
	defer h.StopRegistry(t)

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
		os.Setenv("PACK_HOME", packHome)
	})
	it.After(func() {
		os.Unsetenv("PACK_HOME")
		os.RemoveAll(packHome)
	})

	when("subcommand is invalid", func() {
		it("prints usage", func() {
			cmd := exec.Command(pack, "some-bad-command")
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
		var sourceCodePath, repo, repoName, containerName, registryPort string

		it.Before(func() {
			registryPort = h.RunRegistry(t)
			repo = "some-org/" + h.RandString(10)
			repoName = "localhost:" + registryPort + "/" + repo
			containerName = "test-" + h.RandString(10)

			var err error
			sourceCodePath, err = ioutil.TempDir("", "pack.build.node_app.")
			if err != nil {
				t.Fatal(err)
			}
			h.Run(t, exec.Command("cp", "-r", "testdata/node_app/.", sourceCodePath))

		})
		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ImageRemove(context.TODO(), repoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
			if sourceCodePath != "" {
				os.RemoveAll(sourceCodePath)
			}
		})

		when("'--publish' flag is not specified'", func() {
			it("builds and exports an image", func() {
				cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath)
				h.Run(t, cmd)

				// NOTE: avoid using h.Run as it will convert repoName to include the remote registry port, which is irrelevant given this was done on the daemon
				h.AssertNil(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName).Run())
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(5 * time.Second)
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
				h.Run(t, exec.Command("cp", "-r", "testdata/maven_app/.", sourceCodePath))
			})

			// Skip this test for now. The container run at the very end runs java -jar target/testdata-sample-app-1.0-SNAPSHOT.jar
			// instead of java -jar target/testdata-sample-app-1.0-SNAPSHOT-jar-with-dependencies.jar, which ends
			// up exiting immediately
			it.Pend("assumes latest if no version is provided", func() {
				cmd := exec.Command(pack, "build", repoName, "--buildpack", javaBpId, "-p", sourceCodePath)
				buildOutput := h.Run(t, cmd)

				h.AssertEq(t, strings.Contains(buildOutput, "DETECTING WITH MANUALLY-PROVIDED GROUP:"), true)
				if strings.Contains(buildOutput, "Node.js Buildpack") {
					t.Fatalf("should have skipped Node.js buildpack because --buildpack flag was provided")
				}
				latestInfo := fmt.Sprintf(`No version for '%s' buildpack provided, will use '%s@latest'`, javaBpId, javaBpId)
				if !strings.Contains(buildOutput, latestInfo) {
					t.Fatalf(`expected build output to contain "%s", got "%s"`, latestInfo, buildOutput)
				}
				h.AssertEq(t, strings.Contains(buildOutput, "Sample Java Buildpack: pass"), true)

				h.Run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(2 * time.Second)
				h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Maven buildpack worked!")
			})
		})

		when("'--publish' flag is specified", func() {
			it("builds and exports an image", func() {
				runPackBuild := func() string {
					t.Helper()
					cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath, "--publish")
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

				t.Log("run image:", repoName)
				h.Run(t, exec.Command("docker", "pull", fmt.Sprintf("%s@%s", repoName, imgSHA)))
				h.Run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", fmt.Sprintf("%s@%s", repoName, imgSHA)))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(5 * time.Second)
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
				cmd := exec.Command("docker", "build", "-t", runImage, "-")
				cmd.Stdin = strings.NewReader(fmt.Sprintf("FROM packs/run\nUSER root\nRUN echo %s > /contents1.txt\nRUN echo %s > /contents2.txt\nUSER pack\n", contents1, contents2))
				h.Run(t, cmd)

				h.AssertNil(t, ioutil.WriteFile(filepath.Join(packHome, "config.toml"), []byte(fmt.Sprintf(`
				default-stack-id = "io.buildpacks.stacks.bionic"

				[[stacks]]
				  id = "io.buildpacks.stacks.bionic"
				  build-images = ["packs/build"]
				  run-images = ["%s"]
			`, runImage)), 0666))
			}
			rootContents1 = func() string {
				h.Run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
				launchPort := fetchHostPort(t, containerName)
				time.Sleep(5 * time.Second)
				h.AssertEq(t, h.HttpGet(t, "http://localhost:"+launchPort), "Buildpacks Worked! - 1000:1000")
				txt := h.HttpGet(t, "http://localhost:"+launchPort+"/rootcontents1")
				h.AssertNil(t, dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL"))
				return txt
			}
		})
		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			for _, name := range []string{repoName, runBefore, runAfter} {
				dockerCli.ImageRemove(context.TODO(), name, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
			}
		})

		when("run on daemon", func() {
			it("rebases", func() {
				buildAndSetRunImage(runBefore, "contents-before-1", "contents-before-2")

				cmd := exec.Command(pack, "build", repoName, "-p", "testdata/node_app/", "--no-pull") // , "--publish")
				h.Run(t, cmd)

				h.AssertEq(t, rootContents1(), "contents-before-1\n")

				buildAndSetRunImage(runAfter, "contents-after-1", "contents-after-2")

				cmd = exec.Command(pack, "rebase", repoName, "--no-pull") // , "--publish")
				h.Run(t, cmd)

				h.AssertEq(t, rootContents1(), "contents-after-1\n")
			})
		})

		when("run on registry", func() {
			var registryPort string
			it.Before(func() {
				registryPort = h.RunRegistry(t)

				repoName = "localhost:" + registryPort + "/" + repoName
				runBefore = "localhost:" + registryPort + "/" + runBefore
				runAfter = "localhost:" + registryPort + "/" + runAfter
			})
			it("rebases", func() {
				buildAndSetRunImage(runBefore, "contents-before-1", "contents-before-2")
				h.Run(t, exec.Command("docker", "push", runBefore))

				cmd := exec.Command(pack, "build", repoName, "-p", "testdata/node_app/", "--publish")
				h.Run(t, cmd)

				h.AssertEq(t, rootContents1(), "contents-before-1\n")

				buildAndSetRunImage(runAfter, "contents-after-1", "contents-after-2")
				h.Run(t, exec.Command("docker", "push", runAfter))

				cmd = exec.Command(pack, "rebase", repoName, "--publish")
				h.Run(t, cmd)
				h.Run(t, exec.Command("docker", "pull", repoName))

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
			builderRepoName = "some-org/" + h.RandString(10)
			repoName = "some-org/" + h.RandString(10)
			containerName = "test-" + h.RandString(10)
		})

		it.After(func() {
			dockerCli.ContainerKill(context.TODO(), containerName, "SIGKILL")
			dockerCli.ImageRemove(context.TODO(), builderRepoName, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
		})

		it("builds and exports an image", func() {
			h.AssertNil(t, dockerCli.PullImage("packs/build")) // TODO: control version, 'latest' is not stable across test runs.

			builderTOML := filepath.Join("testdata", "mock_buildpacks", "builder.toml")
			sourceCodePath := filepath.Join("testdata", "mock_app")

			t.Log("create builder image")
			cmd := exec.Command(
				pack, "create-builder",
				builderRepoName,
				"-b", builderTOML,
			)
			h.Run(t, cmd)

			t.Log("build uses order defined in builder.toml")
			cmd = exec.Command(
				pack, "build", repoName,
				"--builder", builderRepoName,
				"--no-pull",
				"--path", sourceCodePath,
			)
			buildOutput, err := cmd.CombinedOutput()
			h.AssertNil(t, err)
			expectedDetectOutput := "First Mock Buildpack: pass | Second Mock Buildpack: pass | Third Mock Buildpack: pass"
			if !strings.Contains(string(buildOutput), expectedDetectOutput) {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, expectedDetectOutput, buildOutput)
			}

			t.Log("run app container")
			cmd = exec.Command("docker", "run", "--name="+containerName, "--rm=true", repoName)
			runOutput := h.Run(t, cmd)
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
			buildOutput, err = cmd.CombinedOutput()
			h.AssertNil(t, err)
			latestInfo := `No version for 'mock.bp.first' buildpack provided, will use 'mock.bp.first@latest'`
			if !strings.Contains(string(buildOutput), latestInfo) {
				t.Fatalf(`expected build output to contain "%s", got "%s"`, latestInfo, buildOutput)
			}
			expectedDetectOutput = "Latest First Mock Buildpack: pass | Third Mock Buildpack: pass"
			if !strings.Contains(string(buildOutput), expectedDetectOutput) {
				t.Fatalf(`Expected build output to contain detection output "%s", got "%s"`, expectedDetectOutput, buildOutput)
			}

			t.Log("run app container")
			cmd = exec.Command("docker", "run", "--name="+containerName, "--rm=true", repoName)
			runOutput = h.Run(t, cmd)
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
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("add-stack command failed: %s: %s", output, err)
			}
		})

		it("updates an existing custom stack in ~/.pack/config.toml", func() {
			cmd := exec.Command(pack, "update-stack", "my.custom.stack", "--run-image", "my-org/run-2", "--run-image", "my-org/run-3", "--build-image", "my-org/build-2", "--build-image", "my-org/build-3")
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
