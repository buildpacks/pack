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
			exec.Command("cp", "-r", "fixtures/node_app/.", sourceCodePath).Run()

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
				cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath, "--detect-image", "packsdev/v3:detect")
				cmd.Env = append(os.Environ(), "HOME="+homeDir)
				run(t, cmd)

				run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
				launchPort := fetchHostPort(t, containerName)

				time.Sleep(2 * time.Second)
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
					cmd := exec.Command(pack, "build", repoName, "-p", sourceCodePath, "--detect-image", "packsdev/v3:detect", "--publish")
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

				time.Sleep(2 * time.Second)
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

	when("create", func() {
		var detectImageName, buildImageName string

		it.Before(func() {
			detectImageName = "some-org/detect-" + randString(10)
			buildImageName = "some-org/build-" + randString(10)
			docker = NewDockerDaemon()
		})
		it.After(func() {
			docker.RemoveImage(detectImageName, buildImageName)
		})

		when("provided with output detect and build images", func() {
			it("creates detect and build images on the daemon", func() {
				cmd := exec.Command(pack, "create", detectImageName, buildImageName, "--from-base-image", "sclevine/test")
				cmd.Env = append(os.Environ(), "HOME="+homeDir)
				cmd.Dir = "./fixtures/buildpacks"
				run(t, cmd)

				t.Log("images exist")
				detectConfig := docker.InspectImage(t, detectImageName)
				buildConfig := docker.InspectImage(t, buildImageName)

				t.Log("both images have buildpacks")
				for _, image := range []string{detectImageName, buildImageName} {
					info := docker.FileFromImage(t, image, "/buildpacks/order.toml")
					if info.Name != "order.toml" || info.Size != 127 {
						t.Fatalf("Expected %s to contain /buildpacks/order.toml: %v", image, info)
					}
				}

				t.Log("both images have ENTRYPOINTs")
				if diff := cmp.Diff(detectConfig.Config.Entrypoint, []string{"/packs/detector"}); diff != "" {
					t.Fatal(diff)
				}
				if diff := cmp.Diff(buildConfig.Config.Entrypoint, []string{"/packs/builder"}); diff != "" {
					t.Fatal(diff)
				}

				t.Log("detect image has desired ENV variables")
				if contains(detectConfig.Config.Env, `"PACK_BP_ORDER_PATH=/buildpacks/order.toml"`) {
					t.Fatalf("Expected %v to contain %s", detectConfig.Config.Env, `"PACK_BP_ORDER_PATH=/buildpacks/order.toml"`)
				}

				t.Log("build image has desired extra ENV variables")
				if contains(buildConfig.Config.Env, `"PACK_METADATA_PATH=/launch/config/metadata.toml"`) {
					t.Fatalf("Expected %v to contain %s", buildConfig.Config.Env, `"PACK_METADATA_PATH=/launch/config/metadata.toml"`)
				}
			})

			when("publishing", func() {
				var registryContainerName, registryPort string
				var registry *DockerRegistry
				it.Before(func() {
					registryContainerName = "test-registry-" + randString(10)
					run(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", registryContainerName, "registry:2"))
					registryPort = fetchHostPort(t, registryContainerName)
					registry = NewDockerRegistry()

					detectImageName = "localhost:" + registryPort + "/" + detectImageName
					buildImageName = "localhost:" + registryPort + "/" + buildImageName
				})
				it.After(func() {
					docker.Kill(registryContainerName)
				})

				it("creates detect and build images on the registry", func() {
					cmd := exec.Command(pack, "create", detectImageName, buildImageName, "--publish", "--from-base-image", "sclevine/test")
					cmd.Env = append(os.Environ(), "HOME="+homeDir)
					cmd.Dir = "./fixtures/buildpacks"
					run(t, cmd)

					t.Log("images exist on registry")
					detectConfig := registry.InspectImage(t, detectImageName)
					buildConfig := registry.InspectImage(t, buildImageName)

					t.Log("both images have ENTRYPOINTs")
					if diff := cmp.Diff(detectConfig.Config.Entrypoint, []string{"/packs/detector"}); diff != "" {
						t.Fatal(diff)
					}
					if diff := cmp.Diff(buildConfig.Config.Entrypoint, []string{"/packs/builder"}); diff != "" {
						t.Fatal(diff)
					}

					t.Log("detect image has desired ENV variables")
					if contains(detectConfig.Config.Env, `"PACK_BP_ORDER_PATH=/buildpacks/order.toml"`) {
						t.Fatalf("Expected %v to contain %s", detectConfig.Config.Env, `"PACK_BP_ORDER_PATH=/buildpacks/order.toml"`)
					}

					t.Log("build image has desired extra ENV variables")
					if contains(buildConfig.Config.Env, `"PACK_METADATA_PATH=/launch/config/metadata.toml"`) {
						t.Fatalf("Expected %v to contain %s", buildConfig.Config.Env, `"PACK_METADATA_PATH=/launch/config/metadata.toml"`)
					}
				})
			})
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))

	when("create, build, run", func() {
		var tmpDir, detectImageName, buildImageName, repoName, containerName, registryContainerName, registry string

		it.Before(func() {
			uid := randString(10)
			registryContainerName = "test-registry-" + uid
			run(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", registryContainerName, "registry:2"))
			registry = "localhost:" + fetchHostPort(t, registryContainerName) + "/"

			var err error
			tmpDir, err = ioutil.TempDir("", "pack.build.node_app.")
			assertNil(t, err)
			assertNil(t, os.Mkdir(filepath.Join(tmpDir, "app"), 0755))
			run(t, exec.Command("cp", "-r", "fixtures/node_app/.", filepath.Join(tmpDir, "app")))
			assertNil(t, os.Mkdir(filepath.Join(tmpDir, "buildpacks"), 0755))
			run(t, exec.Command("cp", "-r", "fixtures/buildpacks/.", filepath.Join(tmpDir, "buildpacks")))

			repoName = registry + "some-org/output-" + uid
			detectImageName = registry + "some-org/detect-" + uid
			buildImageName = registry + "some-org/build-" + uid
			containerName = "test-" + uid

			txt, err := ioutil.ReadFile(filepath.Join(tmpDir, "buildpacks", "order.toml"))
			assertNil(t, err)
			txt2 := strings.Replace(string(txt), "some-build-image", buildImageName, -1)
			txt2 = strings.Replace(txt2, "some-run-image", buildImageName, -1)
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "buildpacks", "order.toml"), []byte(txt2), 0644))
		})
		it.After(func() {
			docker.Kill(containerName, registryContainerName)
			docker.RemoveImage(repoName, detectImageName, buildImageName)
			if tmpDir != "" {
				os.RemoveAll(tmpDir)
			}
		})

		it("run works", func() {
			t.Log("create detect image:")
			cmd := exec.Command(pack, "create", detectImageName, buildImageName, "--from-base-image", "packsdev/v3:latest", "-p", filepath.Join(tmpDir, "buildpacks"), "--publish")
			cmd.Env = append(os.Environ(), "HOME="+homeDir)
			run(t, cmd)

			t.Log("build image from detect:")
			docker.Pull(t, detectImageName, "latest")
			docker.Pull(t, buildImageName, "latest")
			cmd = exec.Command(pack, "build", repoName, "-p", filepath.Join(tmpDir, "app"), "--detect-image", detectImageName, "--publish")
			cmd.Env = append(os.Environ(), "HOME="+homeDir)
			run(t, cmd)

			t.Log("run image:", repoName)
			docker.Pull(t, repoName, "latest")
			run(t, exec.Command("docker", "run", "--name="+containerName, "--rm=true", "-d", "-e", "PORT=8080", "-p", ":8080", repoName))
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
