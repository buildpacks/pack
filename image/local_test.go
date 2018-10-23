package image_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	h "github.com/buildpack/pack/testhelpers"
)

func TestLocal(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "local", testLocal, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLocal(t *testing.T, when spec.G, it spec.S) {
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
		repoName = "pack-image-test-" + h.RandString(10)
	})
	it.After(func() {
		h.RemoveImage(repoName)
	})

	when("#Label", func() {
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL mykey=myvalue other=data
				`)
				h.Run(t, cmd)
			})
			it("returns the label value", func() {
				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)

				label, err := img.Label("mykey")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "myvalue")
			})

			it("returns an empty string for a missing label", func() {
				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)

				label, err := img.Label("missing-label")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "")
			})
		})

		when("image NOT exists", func() {
			it("returns an error", func() {
				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)

				_, err = img.Label("mykey")
				h.AssertError(t, err, fmt.Sprintf("failed to get label, image '%s' does not exist", repoName))
			})
		})
	})

	when("#Name", func() {
		it("always returns the original name", func() {
			img, _ := factory.NewLocal(repoName, false)
			h.AssertEq(t, img.Name(), repoName)
		})
	})

	when("#Digest", func() {
		when("image exists and has a digest", func() {
			var expectedDigest string
			it.Before(func() {
				stdout := h.Run(t, exec.Command("docker", "pull", "busybox:1.29"))
				regex := regexp.MustCompile(`Digest: (sha256:\w*)`)
				matches := regex.FindStringSubmatch(stdout)
				if len(matches) < 2 {
					t.Fatalf("digest regexp failed: %s", stdout)
				}
				expectedDigest = matches[1]
			})

			it("returns the image digest", func() {
				img, _ := factory.NewLocal("busybox:1.29", true)
				digest, err := img.Digest()
				h.AssertNil(t, err)
				h.AssertEq(t, digest, expectedDigest)
			})
		})

		when("image exists but has no digest", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL key=val
				`)
				h.Run(t, cmd)
			})

			it.After(func() {
				h.RemoveImage(repoName)
			})

			it("returns an empty string", func() {
				img, _ := factory.NewLocal(repoName, false)
				digest, err := img.Digest()
				h.AssertNil(t, err)
				h.AssertEq(t, digest, "")
			})
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
				h.Run(t, cmd)
			})
			it("sets label on img object", func() {
				img, _ := factory.NewLocal(repoName, false)
				h.AssertNil(t, img.SetLabel("mykey", "new-val"))
				label, err := img.Label("mykey")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "new-val")
			})

			it("saves label to docker daemon", func() {
				img, _ := factory.NewLocal(repoName, false)
				h.AssertNil(t, img.SetLabel("mykey", "new-val"))
				_, err := img.Save()
				h.AssertNil(t, err)

				label := h.Run(t, exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`))
				h.AssertEq(t, strings.TrimSpace(label), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase string
			it.Before(func() {
				oldBase = "pack-oldbase-test-" + h.RandString(10)
				oldTopLayer = createImageOnLocal(t, oldBase, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				newBase = "pack-newbase-test-" + h.RandString(10)
				createImageOnLocal(t, newBase, `
					FROM busybox
					RUN echo new-base > base.txt
					RUN echo text-new-base > otherfile.txt
				`)

				createImageOnLocal(t, repoName, fmt.Sprintf(`
					FROM %s
					RUN echo text-from-image > myimage.txt
					RUN echo text-from-image > myimage2.txt
				`, oldBase))
			})
			it.After(func() {
				h.RemoveImage(oldBase, newBase)
			})

			it("switches the base", func() {
				// Before
				txt := h.Run(t, exec.Command("docker", "run", repoName, "cat", "base.txt"))
				h.AssertEq(t, txt, "old-base\n")

				// Run rebase
				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)
				newBaseImg, err := factory.NewLocal(newBase, false)
				h.AssertNil(t, err)
				err = img.Rebase(oldTopLayer, newBaseImg)
				h.AssertNil(t, err)
				_, err = img.Save()
				h.AssertNil(t, err)

				// After
				txt = h.Run(t, exec.Command("docker", "run", repoName, "cat", "base.txt"))
				h.AssertEq(t, txt, "new-base\n")
			})
		})
	})

	when("#TopLayer", func() {
		when("image exists", func() {
			it("returns the digest for the top layer (useful for rebasing)", func() {
				expectedTopLayer := createImageOnLocal(t, repoName, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				img, err := factory.NewLocal(repoName, false)
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
				createImageOnLocal(t, repoName, `
					FROM busybox
					LABEL mykey=oldValue
				`)

				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)

				err = img.SetLabel("mykey", "newValue")
				h.AssertNil(t, err)

				imgDigest, err := img.Save()
				h.AssertNil(t, err)

				label := h.Run(t, exec.Command("docker", "inspect", imgDigest, "-f", `{{.Config.Labels.mykey}}`))
				h.AssertEq(t, strings.TrimSpace(label), "newValue")
			})
		})
	})
}

func createImageOnLocal(t *testing.T, repoName, dockerFile string) string {
	t.Helper()

	cmd := exec.Command("docker", "build", "-t", repoName+":latest", "-")
	cmd.Stdin = strings.NewReader(dockerFile)
	h.Run(t, cmd)

	topLayerJSON := h.Run(t, exec.Command("docker", "inspect", repoName, "-f", `{{json .RootFS.Layers}}`))
	var layers []string
	h.AssertNil(t, json.Unmarshal([]byte(topLayerJSON), &layers))
	topLayer := layers[len(layers)-1]

	return topLayer
}
