package image_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var registryPort string

func TestRemote(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	registryPort = h.RunRegistry(t, false)
	defer h.StopRegistry(t)
	registryPort = h.RunRegistry(t, false)

	spec.Run(t, "remote", testRemote, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRemote(t *testing.T, when spec.G, it spec.S) {
	var factory image.Factory
	var buf bytes.Buffer
	var repoName string

	it.Before(func() {
		docker, err := docker.New()
		h.AssertNil(t, err)
		factory = image.Factory{
			Docker: docker,
			Log:    log.New(&buf, "", log.LstdFlags),
			Stdout: &buf,
			FS:     &fs.FS{},
		}
		repoName = "localhost:" + registryPort + "/pack-image-test-" + h.RandString(10)
	})

	when("#Label", func() {
		when("image exists", func() {
			var img image.Image
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				h.Run(t, cmd)
				h.Run(t, exec.Command("docker", "push", repoName))
				h.Run(t, exec.Command("docker", "rmi", repoName))

				var err error
				img, err = factory.NewRemote(repoName)
				h.AssertNil(t, err)
			})

			it("returns the label value", func() {
				label, err := img.Label("mykey")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "myvalue")
			})

			it("returns an empty string for a missing label", func() {
				label, err := img.Label("missing-label")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "")
			})
		})

		when("image NOT exists", func() {
			it("returns an error", func() {
				img, err := factory.NewRemote(repoName)
				h.AssertNil(t, err)

				_, err = img.Label("mykey")
				h.AssertError(t, err, fmt.Sprintf("failed to get label, image '%s' does not exist", repoName))
			})
		})
	})

	when("#Name", func() {
		it("always returns the original name", func() {
			img, _ := factory.NewRemote(repoName)
			h.AssertEq(t, img.Name(), repoName)
		})
	})

	when("#Digest", func() {
		it("returns the image digest", func() {
			//busybox:1.29 has digest sha256:915f390a8912e16d4beb8689720a17348f3f6d1a7b659697df850ab625ea29d5
			img, _ := factory.NewRemote("busybox:1.29")
			digest, err := img.Digest()
			h.AssertNil(t, err)
			h.AssertEq(t, digest, "sha256:915f390a8912e16d4beb8689720a17348f3f6d1a7b659697df850ab625ea29d5")
		})
	})

	when("#SetLabel", func() {
		var img image.Image
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				h.Run(t, cmd)
				h.Run(t, exec.Command("docker", "push", repoName))
				h.Run(t, exec.Command("docker", "rmi", repoName))

				var err error
				img, err = factory.NewRemote(repoName)
				h.AssertNil(t, err)
			})

			it("sets label on img object", func() {
				h.AssertNil(t, img.SetLabel("mykey", "new-val"))
				label, err := img.Label("mykey")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "new-val")
			})

			it("saves label", func() {
				h.AssertNil(t, img.SetLabel("mykey", "new-val"))
				_, err := img.Save()
				h.AssertNil(t, err)

				// After Pull
				h.Run(t, exec.Command("docker", "pull", repoName))
				defer h.Run(t, exec.Command("docker", "rmi", repoName))
				label := h.Run(t, exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`))
				h.AssertEq(t, strings.TrimSpace(label), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase string
			it.Before(func() {
				oldBase = "localhost:" + registryPort + "/pack-oldbase-test-" + h.RandString(10)
				oldTopLayer = createImageOnRemote(t, oldBase, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				newBase = "localhost:" + registryPort + "/pack-newbase-test-" + h.RandString(10)
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

			it("switches the base", func() {
				// Before
				txt := h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "base.txt"))
				h.AssertEq(t, txt, "old-base\n")

				// Run rebase
				img, err := factory.NewRemote(repoName)
				h.AssertNil(t, err)
				newBaseImg, err := factory.NewRemote(newBase)
				h.AssertNil(t, err)
				err = img.Rebase(oldTopLayer, newBaseImg)
				h.AssertNil(t, err)
				_, err = img.Save()
				h.AssertNil(t, err)

				// After
				h.Run(t, exec.Command("docker", "pull", repoName))
				txt = h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "base.txt"))
				h.AssertEq(t, txt, "new-base\n")
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
				h.AssertNil(t, err)

				actualTopLayer, err := img.TopLayer()
				h.AssertNil(t, err)

				h.AssertEq(t, actualTopLayer, expectedTopLayer)
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
				h.AssertNil(t, err)

				err = img.SetLabel("mykey", "newValue")
				h.AssertNil(t, err)

				imgDigest, err := img.Save()
				h.AssertNil(t, err)

				// After Pull
				defer h.Run(t, exec.Command("docker", "rmi", repoName+"@"+imgDigest))
				h.Run(t, exec.Command("docker", "pull", repoName+"@"+imgDigest))
				label := h.Run(t, exec.Command("docker", "inspect", repoName+"@"+imgDigest, "-f", `{{.Config.Labels.mykey}}`))
				h.AssertEq(t, strings.TrimSpace(label), "newValue")
			})
		})
	})
}

func createImageOnRemote(t *testing.T, repoName, dockerFile string) string {
	t.Helper()
	defer h.Run(t, exec.Command("docker", "rmi", repoName))

	cmd := exec.Command("docker", "build", "-t", repoName+":latest", "-")
	cmd.Stdin = strings.NewReader(dockerFile)
	h.Run(t, cmd)

	topLayerJSON := h.Run(t, exec.Command("docker", "inspect", repoName, "-f", `{{json .RootFS.Layers}}`))
	var layers []string
	h.AssertNil(t, json.Unmarshal([]byte(topLayerJSON), &layers))
	topLayer := layers[len(layers)-1]

	h.Run(t, exec.Command("docker", "push", repoName))

	return topLayer
}
