package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var pack string

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

	NewDockerDaemon().Pull(t, "registry", "2")
	NewDockerDaemon().Pull(t, "sclevine/test", "latest")

	spec.Run(t, "pack", testPack, spec.Report(report.Terminal{}))
}

func testPack(t *testing.T, when spec.G, it spec.S) {
	var homeDir string
	var docker *DockerDaemon

	it.Before(func() {
		if _, err := os.Stat(pack); os.IsNotExist(err) {
			t.Fatal("No file found at PACK_PATH environment variable:", pack)
		}
		docker = NewDockerDaemon()

		var err error
		homeDir, err = ioutil.TempDir("", "buildpack.pack.build.homedir.")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(homeDir, ".docker"), 0777); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(homeDir, ".docker", "config.json"), []byte("{}"), 0666); err != nil {
			t.Fatal(err)
		}
	})
	it.After(func() {
		os.RemoveAll(homeDir)
	})

	when("subcommand is invalid", func() {
		it("prints usage", func() {
			cmd := exec.Command(pack, "some-bad-command")
			cmd.Env = append(os.Environ(), "HOME="+homeDir)
			output, _ := cmd.CombinedOutput()
			if !strings.Contains(string(output), `unknown command "some-bad-command" for "pack"`) {
				t.Fatal("Failed to print usage", string(output))
			}
			if !strings.Contains(string(output), `Run 'pack --help' for usage.`) {
				t.Fatal("Failed to print usage", string(output))
			}
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("build on daemon", func() {
		var sourceCodePath, repo, repoName, containerName, registryContainerName, registryPort string

		it.Before(func() {
			registryContainerName = "test-registry-" + randString(10)
			run(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", registryContainerName, "registry:2"))
			registryPort = fetchHostPort(t, registryContainerName)

			var err error
			sourceCodePath, err = ioutil.TempDir("", "pack.build.node_app.")
			if err != nil {
				t.Fatal(err)
			}
			exec.Command("cp", "-r", "testdata/node_app/.", sourceCodePath).Run()

			repo = "some-org/" + randString(10)
			repoName = "localhost:" + registryPort + "/" + repo
			containerName = "test-" + randString(10)
		})
		it.After(func() {
			docker.Kill(containerName, registryContainerName)
			docker.RemoveImage(repoName)
			if sourceCodePath != "" {
				os.RemoveAll(sourceCodePath)
			}
		})

		when("'--publish' flag is not specified'", func() {
			it("builds and exports an image", func() {
				cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath)
				cmd.Env = append(os.Environ(), "HOME="+homeDir)
				run(t, cmd)

				run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(5 * time.Second)
				assertEq(t, fetch(t, "http://localhost:"+launchPort), "Buildpacks Worked!")

				t.Log("Checking that registry is empty")
				contents := fetch(t, fmt.Sprintf("http://localhost:%s/v2/_catalog", registryPort))
				if strings.Contains(string(contents), repo) {
					t.Fatalf("Should not have published image without the '--publish' flag: got %s", contents)
				}
			})
		}, spec.Parallel(), spec.Report(report.Terminal{}))

		when("'--publish' flag is specified", func() {
			it("builds and exports an image", func() {
				runPackBuild := func() string {
					t.Helper()
					cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath, "--publish")
					cmd.Env = append(os.Environ(), "HOME="+homeDir)
					return run(t, cmd)
				}
				output := runPackBuild()
				imgSHA, err := imgSHAFromOutput(output, repoName)
				if err != nil {
					fmt.Println(output)
					t.Fatal("Could not determine sha for built image")
				}

				t.Log("Checking that registry has contents")
				contents := fetch(t, fmt.Sprintf("http://localhost:%s/v2/_catalog", registryPort))
				if !strings.Contains(string(contents), repo) {
					t.Fatalf("Expected to see image %s in %s", repo, contents)
				}

				t.Log("run image:", repoName)
				docker.Pull(t, repoName, imgSHA)
				run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", fmt.Sprintf("%s@%s", repoName, imgSHA)))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(5 * time.Second)
				assertEq(t, fetch(t, "http://localhost:"+launchPort), "Buildpacks Worked!")

				t.Log("uses the cache on subsequent run")
				output = runPackBuild()

				regex := regexp.MustCompile(`moved \d+ packages`)
				if !regex.MatchString(output) {
					t.Fatalf("Build failed to use cache: %s", output)
				}
			})
		}, spec.Parallel(), spec.Report(report.Terminal{}))
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("create-builder", func() {
		var (
			builderTOML     string
			builderRepoName string
			appRepoName     string
			containerName   string
			tmpDir          string
		)

		it.Before(func() {
			builderTOML = filepath.Join("testdata", "builder.toml")

			var err error
			tmpDir, err = ioutil.TempDir("", "pack.build.node_app.")
			assertNil(t, err)
			assertNil(t, os.Mkdir(filepath.Join(tmpDir, "app"), 0755))
			run(t, exec.Command("cp", "-r", "testdata/node_app/.", filepath.Join(tmpDir, "app")))

			builderRepoName = "some-org/" + randString(10)
			appRepoName = "some-org/" + randString(10)
			containerName = "test-" + randString(10)
		})
		it.After(func() {
			docker.Kill(containerName)
			docker.RemoveImage(builderRepoName, appRepoName)
			if tmpDir != "" {
				os.RemoveAll(tmpDir)
			}
		})

		it("creates a builder image", func() {
			t.Log("create builder image")
			cmd := exec.Command(pack, "create-builder", builderRepoName, "-b", builderTOML)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("create-builder command failed: %s: %s", output, err)
			}

			t.Log("builder image has order toml and buildpacks")
			dockerRunOutput := run(t, exec.Command("docker", "run", "--rm=true", "-t", builderRepoName, "ls", "/buildpacks"))

			if !strings.Contains(dockerRunOutput, "order.toml") {
				t.Fatalf("expected /buildpacks to contain order.toml, got '%s'", dockerRunOutput)
			}
			if !strings.Contains(dockerRunOutput, "com.example.sample.bp") {
				t.Fatalf("expected /buildpacks to contain com.example.sample.bp, got '%s'", dockerRunOutput)
			}

			dockerRunOutput = run(t, exec.Command("docker", "run", "--rm=true", "-t", builderRepoName, "cat", "/buildpacks/order.toml"))
			sanitzedOutput := strings.Replace(dockerRunOutput, "\r", "", -1)
			expectedGroups := `[[groups]]

  [[groups.buildpacks]]
    id = "com.example.sample.bp"
    version = "1.2.3"
`

			if diff := cmp.Diff(sanitzedOutput, expectedGroups); diff != "" {
				t.Fatalf("expected order.toml to contain '%s', got diff '%s'", expectedGroups, diff)
			}

			t.Log("build app with builder:", builderRepoName)
			NewDockerDaemon().Pull(t, "packs/run", "latest")
			cmd = exec.Command(pack, "build", appRepoName, "-p", filepath.Join(tmpDir, "app"), "--builder", builderRepoName, "--no-pull", "--run-image", "packs/run")
			cmd.Env = append(os.Environ(), "HOME="+homeDir)
			run(t, cmd)

			t.Log("run image:", appRepoName)
			txt := run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", appRepoName))
			if !strings.Contains(txt, "Hi from Sample BP") {
				t.Fatalf("expected '%s' to be contained in:\n%s", "Hi from Sample BP", txt)
			}
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))
}

func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute command: %v, %s, %s", cmd.Args, err, output)
	}

	return string(output)
}

func fetch(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to make request to [%s]: %s", url, err)
	}

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to make request to [%s]: %s", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Request returned bad status code: [%d] : %s", resp.StatusCode, contents)
	}

	return string(contents)
}

func fetchHostPort(t *testing.T, dockerID string) string {
	t.Helper()

	body, _, err := NewDockerDaemon().Do("GET", fmt.Sprintf("/containers/%s/json", dockerID), nil, nil)
	if err != nil {
		t.Fatalf("Failed to fetch host port for %s: %s", dockerID, err)
	}
	var out struct {
		NetworkSettings struct {
			Ports map[string][]struct {
				HostPort string
			}
		}
	}
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		t.Fatalf("Failed to fetch host port for %s: %s", dockerID, err)
	}
	for _, p := range out.NetworkSettings.Ports {
		if len(p) > 0 {
			return p[0].HostPort
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

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func contains(arr []string, val string) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}
