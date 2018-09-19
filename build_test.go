package pack_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/docker"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestBuild(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	assertNil(t, exec.Command("docker", "pull", "registry:2").Run())
	assertNil(t, exec.Command("docker", "pull", "packs/samples").Run())
	assertNil(t, exec.Command("docker", "pull", "packs/run").Run())
	spec.Run(t, "build", testBuild, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var subject *pack.BuildFlags
	var tmpDir string
	var buf bytes.Buffer

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("/tmp", "pack.build.")
		assertNil(t, err)
		subject = &pack.BuildFlags{
			AppDir:          "acceptance/testdata/node_app",
			Builder:         "packs/samples",
			RunImage:        "packs/run",
			RepoName:        "pack.build." + randString(10),
			Publish:         false,
			WorkspaceVolume: filepath.Join(tmpDir, "workspace"),
			CacheVolume:     filepath.Join(tmpDir, "cache"),
			Stdout:          &buf,
			Stderr:          &buf,
			Log:             log.New(&buf, "", log.LstdFlags|log.Lshortfile),
		}
		log.SetOutput(ioutil.Discard)
		subject.Cli, err = docker.New()
		assertNil(t, err)
		assertNil(t, os.MkdirAll(filepath.Join(tmpDir, "workspace", "app"), 0777))
		assertNil(t, os.MkdirAll(filepath.Join(tmpDir, "cache"), 0777))
	})
	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Detect", func() {
		when("app is detected", func() {
			it("returns the successful group with node", func() {
				group, err := subject.Detect()
				assertNil(t, err)
				assertEq(t, group.Buildpacks[0].ID, "io.buildpacks.samples.nodejs")
			})
		})

		when("app is not detectable", func() {
			it.Before(func() {
				assertNil(t, os.Mkdir(filepath.Join(tmpDir, "badapp"), 0777))
				assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "badapp", "file.txt"), []byte("content"), 0644))
				subject.AppDir = filepath.Join(tmpDir, "badapp")
			})
			it("returns the successful group with node", func() {
				_, err := subject.Detect()

				assertNotNil(t, err)
				assertEq(t, err.Error(), "run detect container: failed with status code: 6")
			})
		})
	})

	when("#Analyze", func() {
		it.Before(func() {
			assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "workspace", "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
				version = "0.0.1"
			`), 0666))
		})
		when("no previous image exists", func() {
			when("publish", func() {
				var registryContainerName, registryPort string
				it.Before(func() {
					registryContainerName, registryPort = runRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})
				it.After(func() { assertNil(t, exec.Command("docker", "kill", registryContainerName).Run()) })

				it("informs the user", func() {
					err := subject.Analyze()
					assertNil(t, err)
					assertContains(t, buf.String(), "WARNING: skipping analyze, image not found or requires authentication to access:")
				})
			})
			when("daemon", func() {
				it.Before(func() { subject.Publish = false })
				it("informs the user", func() {
					err := subject.Analyze()
					assertNil(t, err)
					assertContains(t, buf.String(), "WARNING: skipping analyze, image not found\n")
				})
			})
		})

		when("previous image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", subject.RepoName, "-")
				cmd.Stdin = strings.NewReader("FROM scratch\n" + `LABEL io.buildpacks.lifecycle.metadata='{"buildpacks":[{"key":"io.buildpacks.samples.nodejs","layers":{"node_modules":{"sha":"sha256:99311ec03d790adf46d35cd9219ed80a7d9a4b97f761247c02c77e7158a041d5","data":{"lock_checksum":"eb04ed1b461f1812f0f4233ef997cdb5"}}}}]}'` + "\n")
				assertNil(t, cmd.Run())
			})
			it.After(func() {
				exec.Command("docker", "rmi", subject.RepoName).Run()
			})

			when("publish", func() {
				var registryContainerName, registryPort string
				it.Before(func() {
					oldRepoName := subject.RepoName
					registryContainerName, registryPort = runRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true

					assertNil(t, exec.Command("docker", "tag", oldRepoName, subject.RepoName).Run())
					assertNil(t, exec.Command("docker", "push", subject.RepoName).Run())
					assertNil(t, exec.Command("docker", "rmi", oldRepoName, subject.RepoName).Run())
				})
				it.After(func() {
					assertNil(t, exec.Command("docker", "kill", registryContainerName).Run())
				})

				it("tells the user nothing", func() {
					assertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					assertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					assertNil(t, subject.Analyze())

					txt, err := ioutil.ReadFile(filepath.Join(tmpDir, "workspace", "io.buildpacks.samples.nodejs", "node_modules.toml"))
					assertNil(t, err)
					assertEq(t, string(txt), "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})

			when("daemon", func() {
				it.Before(func() { subject.Publish = false })

				it("tells the user nothing", func() {
					assertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					assertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					assertNil(t, subject.Analyze())

					txt, err := ioutil.ReadFile(filepath.Join(tmpDir, "workspace", "io.buildpacks.samples.nodejs", "node_modules.toml"))
					assertNil(t, err)
					assertEq(t, string(txt), "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})
		})
	})

	when("#Export", func() {
		var group *lifecycle.BuildpackGroup
		it.Before(func() {
			files := map[string]string{
				"group.toml":           "[[buildpacks]]\n" + `id = "io.buildpacks.samples.nodejs"` + "\n" + `version = "0.0.1"`,
				"app/file.txt":         "some text",
				"config/metadata.toml": "stuff = \"text\"",
				"io.buildpacks.samples.nodejs/mylayer.toml":     `key = "myval"`,
				"io.buildpacks.samples.nodejs/mylayer/file.txt": "content",
				"io.buildpacks.samples.nodejs/other.toml":       "",
				"io.buildpacks.samples.nodejs/other/file.txt":   "something",
			}
			for name, txt := range files {
				assertNil(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, "workspace", name)), 0777))
				assertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "workspace", name), []byte(txt), 0666))
			}

			group = &lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "io.buildpacks.samples.nodejs", Version: "0.0.1"},
				},
			}
		})
		it.After(func() { exec.Command("docker", "rmi", subject.RepoName).Run() })

		when("no previous image exists", func() {
			when("publish", func() {
				var oldRepoName, registryContainerName, registryPort string
				it.Before(func() {
					oldRepoName = subject.RepoName
					registryContainerName, registryPort = runRegistry(t)
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})
				it.After(func() {
					assertNil(t, exec.Command("docker", "kill", registryContainerName).Run())
				})
				it("creates the image on the registry", func() {
					assertNil(t, subject.Export(group))
					images := httpGet(t, "http://localhost:"+registryPort+"/v2/_catalog")
					assertContains(t, images, oldRepoName)
				})
				it("puts the files on the image", func() {
					assertNil(t, subject.Export(group))

					assertNil(t, exec.Command("docker", "pull", subject.RepoName).Run())
					txt, err := exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/app/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "some text")

					txt, err = exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/io.buildpacks.samples.nodejs/mylayer/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "content")
				})
				it("sets the metadata on the image", func() {
					assertNil(t, subject.Export(group))

					assertNil(t, exec.Command("docker", "pull", subject.RepoName).Run())
					var metadata lifecycle.AppImageMetadata
					metadataJSON, err := exec.Command("docker", "inspect", subject.RepoName, "--format", `{{index .Config.Labels "io.buildpacks.lifecycle.metadata"}}`).Output()
					assertNil(t, err)
					assertNil(t, json.Unmarshal(metadataJSON, &metadata))

					assertContains(t, metadata.App.SHA, "sha256:")
					assertContains(t, metadata.Config.SHA, "sha256:")
					assertEq(t, len(metadata.Buildpacks), 1)
					assertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
					assertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
					assertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
				})
			})

			when("daemon", func() {
				it.Before(func() { subject.Publish = false })
				it("creates the image on the daemon", func() {
					assertNil(t, subject.Export(group))
					images, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}").Output()
					assertNil(t, err)
					assertContains(t, string(images), subject.RepoName)
				})
				it("puts the files on the image", func() {
					assertNil(t, subject.Export(group))

					txt, err := exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/app/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "some text")

					txt, err = exec.Command("docker", "run", subject.RepoName, "cat", "/workspace/io.buildpacks.samples.nodejs/mylayer/file.txt").Output()
					assertNil(t, err)
					assertEq(t, string(txt), "content")
				})
				it("sets the metadata on the image", func() {
					assertNil(t, subject.Export(group))

					var metadata lifecycle.AppImageMetadata
					metadataJSON, err := exec.Command("docker", "inspect", subject.RepoName, "--format", `{{index .Config.Labels "io.buildpacks.lifecycle.metadata"}}`).Output()
					assertNil(t, err)
					assertNil(t, json.Unmarshal(metadataJSON, &metadata))

					assertEq(t, metadata.RunImage.Name, "packs/run")
					assertContains(t, metadata.App.SHA, "sha256:")
					assertContains(t, metadata.Config.SHA, "sha256:")
					assertEq(t, len(metadata.Buildpacks), 1)
					assertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
					assertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
					assertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
				})
			})
		})

		when("previous image exists", func() {
			it("reuses images from previous layers", func() {
				addLayer := "ADD --chown=pack:pack /workspace/io.buildpacks.samples.nodejs/mylayer /workspace/io.buildpacks.samples.nodejs/mylayer"
				copyLayer := "COPY --from=prev --chown=pack:pack /workspace/io.buildpacks.samples.nodejs/mylayer /workspace/io.buildpacks.samples.nodejs/mylayer"

				assertNil(t, subject.Export(group))
				assertContains(t, buf.String(), addLayer)

				buf.Reset()
				assertNil(t, os.RemoveAll(filepath.Join(tmpDir, "workspace", "io.buildpacks.samples.nodejs", "mylayer")))

				assertNil(t, subject.Export(group))
				assertContains(t, buf.String(), copyLayer)
			})
		})
	})

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

func assertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' inside '%s'", expected, actual)
	}
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func assertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual == nil {
		t.Fatal("Expected not nil")
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

func runRegistry(t *testing.T) (string, string) {
	t.Helper()
	name := "test-registry-" + randString(10)
	assertNil(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", name, "registry:2").Run())
	port, err := exec.Command("docker", "inspect", name, "-f", `{{index (index (index .NetworkSettings.Ports "5000/tcp") 0) "HostPort"}}`).Output()
	assertNil(t, err)
	return name, strings.TrimSpace(string(port))
}

func httpGet(t *testing.T, url string) string {
	resp, err := http.Get(url)
	assertNil(t, err)
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		t.Fatalf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	assertNil(t, err)
	return string(b)
}
