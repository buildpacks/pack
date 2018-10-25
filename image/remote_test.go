package image_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var registryPort string

func TestRemote(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	log.SetOutput(ioutil.Discard)

	registryContainerName := "test-registry-" + randString(10)
	defer exec.Command("docker", "kill", registryContainerName).Run()
	run(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", registryContainerName, "registry:2"))
	b, err := exec.Command("docker", "inspect", registryContainerName, "-f", `{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}`).Output()
	assertNil(t, err)
	registryPort = strings.TrimSpace(string(b))

	spec.Run(t, "remote", testRemote, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRemote(t *testing.T, when spec.G, it spec.S) {
	var factory image.Factory
	var buf bytes.Buffer
	var repoName string

	it.Before(func() {
		docker, err := docker.New()
		assertNil(t, err)
		factory = image.Factory{
			Docker: docker,
			Log:    log.New(&buf, "", log.LstdFlags),
			Stdout: &buf,
			FS:     &fs.FS{},
		}
		repoName = "localhost:" + registryPort + "/pack-image-test-" + randString(10)
	})
	it.After(func() {
		exec.Command("docker", "rmi", "-f", repoName).Run()
		exec.Command("bash", "-c", fmt.Sprintf(`docker rmi -f $(docker images --format='{{.ID}}' %s)`, repoName)).Run()
	})

	when("#Label", func() {
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				assertNil(t, cmd.Run())
				run(t, exec.Command("docker", "push", repoName))
				run(t, exec.Command("docker", "rmi", repoName))
			})

			it("returns the label value", func() {
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "myvalue")
			})

			it("returns an empty string for a missing label", func() {
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				label, err := img.Label("missing-label")
				assertNil(t, err)
				assertEq(t, label, "")
			})
		})

		when("image NOT exists", func() {
			it("returns an error", func() {
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				_, err = img.Label("mykey")
				assertError(t, err, fmt.Sprintf("failed to get label, image '%s' does not exist", repoName))
			})
		})
	})

	when("#Name", func() {
		it("always returns the original name", func() {
			img, _ := factory.NewRemote(repoName)
			assertEq(t, img.Name(), repoName)
		})
	})

	when("#Digest", func() {
		it("returns the image digest", func() {
			//busybox:1.29 has digest sha256:915f390a8912e16d4beb8689720a17348f3f6d1a7b659697df850ab625ea29d5
			img, _ := factory.NewRemote("busybox:1.29")
			digest, err := img.Digest()
			assertNil(t, err)
			assertEq(t, digest, "sha256:915f390a8912e16d4beb8689720a17348f3f6d1a7b659697df850ab625ea29d5")
		})
	})

	when("#SetLabel", func() {
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				assertNil(t, cmd.Run())
				run(t, exec.Command("docker", "push", repoName))
				run(t, exec.Command("docker", "rmi", repoName))
			})
			it.After(func() {
				exec.Command("docker", "rmi", repoName).Run()
			})

			it("sets label on img object", func() {
				img, _ := factory.NewRemote(repoName)
				assertNil(t, img.SetLabel("mykey", "new-val"))
				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "new-val")
			})

			it("saves label to docker daemon", func() {
				img, _ := factory.NewRemote(repoName)
				assertNil(t, img.SetLabel("mykey", "new-val"))
				_, err := img.Save()
				assertNil(t, err)

				// Before Pull
				label, err := exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "")

				// After Pull
				run(t, exec.Command("docker", "pull", repoName))
				label, err = exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase string
			it.Before(func() {
				oldBase = "localhost:" + registryPort + "/pack-oldbase-test-" + randString(10)
				oldTopLayer = createImageOnRemote(t, oldBase, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				newBase = "localhost:" + registryPort + "/pack-newbase-test-" + randString(10)
				createImageOnRemote(t, newBase, `
					FROM busybox
					RUN echo new-base > base.txt
					RUN echo text-new-base > otherfile.txt
				`)

				createImageOnRemote(t, repoName, fmt.Sprintf(`
					FROM %s
					RUN echo text-from-image > myimage.txt
					RUN echo text-from-image > myimage2.txt
				`, oldBase))
			})
			it.After(func() {
				exec.Command("docker", "rmi", oldBase, newBase).Run()
			})

			it("switches the base", func() {
				// Before
				txt, err := exec.Command("docker", "run", repoName, "cat", "base.txt").Output()
				assertNil(t, err)
				assertEq(t, string(txt), "old-base\n")

				// Run rebase
				img, err := factory.NewRemote(repoName)
				assertNil(t, err)
				newBaseImg, err := factory.NewRemote(newBase)
				assertNil(t, err)
				err = img.Rebase(oldTopLayer, newBaseImg)
				assertNil(t, err)
				_, err = img.Save()
				assertNil(t, err)

				// After
				run(t, exec.Command("docker", "pull", repoName))
				txt, err = exec.Command("docker", "run", repoName, "cat", "base.txt").Output()
				assertNil(t, err)
				assertEq(t, string(txt), "new-base\n")
			})
		})
	})

	when("#TopLayer", func() {
		when("image exists", func() {
			it("returns the digest for the top layer (useful for rebasing)", func() {
				expectedTopLayer := createImageOnRemote(t, repoName, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				actualTopLayer, err := img.TopLayer()
				assertNil(t, err)

				assertEq(t, actualTopLayer, expectedTopLayer)
			})
		})
	})

	when("#Save", func() {
		when("image exists", func() {
			it("returns the image digest", func() {
				createImageOnRemote(t, repoName, `
					FROM busybox
					LABEL mykey=oldValue
				`)

				img, err := factory.NewRemote(repoName)
				assertNil(t, err)

				err = img.SetLabel("mykey", "newValue")
				assertNil(t, err)

				imgDigest, err := img.Save()
				assertNil(t, err)

				// After Pull
				defer exec.Command("docker", "rmi", repoName+"@"+imgDigest).Run()
				run(t, exec.Command("docker", "pull", repoName+"@"+imgDigest))
				label, err := exec.Command("docker", "inspect", repoName+"@"+imgDigest, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "newValue")
			})
		})
	})
}

func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute command: %v, %s, %s", cmd.Args, err, output)
	}

	return string(output)
}

func createImageOnRemote(t *testing.T, repoName, dockerFile string) string {
	t.Helper()
	defer exec.Command("docker", "rmi", repoName+":latest")

	cmd := exec.Command("docker", "build", "-t", repoName+":latest", "-")
	cmd.Stdin = strings.NewReader(dockerFile)
	run(t, cmd)

	topLayerJSON, err := exec.Command("docker", "inspect", repoName, "-f", `{{json .RootFS.Layers}}`).Output()
	assertNil(t, err)
	var layers []string
	assertNil(t, json.Unmarshal(topLayerJSON, &layers))
	topLayer := layers[len(layers)-1]

	run(t, exec.Command("docker", "push", repoName))

	return topLayer
}
