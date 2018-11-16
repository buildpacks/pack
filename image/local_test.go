package image_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/buildpack/pack/docker"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
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

	when("#Label", func() {
		when("image exists", func() {
			it.Before(func() {
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(fmt.Sprintf(`
					FROM scratch
					LABEL repo_name_for_randomisation=%s
					LABEL mykey=myvalue other=data
				`, repoName))
				h.Run(t, cmd)
			})

			it.After(func() {
				h.Run(t, exec.Command("docker", "rmi", repoName))
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
			img, err := factory.NewLocal(repoName, false)
			h.AssertNil(t, err)

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
				h.Run(t, exec.Command("docker", "rmi", repoName))
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
			var (
				img    image.Image
				origID string
			)
			it.Before(func() {
				var err error
				cmd := exec.Command("docker", "build", "-t", repoName, "-")
				cmd.Stdin = strings.NewReader(`
					FROM scratch
					LABEL some-key=some-value
				`)
				h.Run(t, cmd)
				img, err = factory.NewLocal(repoName, false)
				h.AssertNil(t, err)
				origID = h.ImageID(t, repoName)
			})

			it.After(func() {
				h.Run(t, exec.Command("docker", "rmi", repoName))
				h.Run(t, exec.Command("docker", "rmi", origID))
			})

			it("sets label and saves label to docker daemon", func() {
				h.AssertNil(t, img.SetLabel("somekey", "new-val"))
				t.Log("set label")
				label, err := img.Label("somekey")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "new-val")
				t.Log("save label")
				_, err = img.Save()
				h.AssertNil(t, err)

				label = h.Run(t, exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.somekey}}`))
				h.AssertEq(t, strings.TrimSpace(label), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase, origNumLayers, origID string
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

				origNumLayers = h.Run(t, exec.Command("docker", "inspect", repoName, "-f", "{{len .RootFS.Layers}}"))
				origID = h.ImageID(t, repoName)
			})

			it.After(func() {
				h.Run(t, exec.Command("docker", "rmi", repoName))
				h.Run(t, exec.Command("docker", "rmi", oldBase))
				h.Run(t, exec.Command("docker", "rmi", newBase))
				h.Run(t, exec.Command("docker", "rmi", origID))
			})

			it("switches the base", func() {
				// Before
				txt := h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "base.txt"))
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
				expected := map[string]string{
					"base.txt":      "new-base\n",
					"otherfile.txt": "text-new-base\n",
					"myimage.txt":   "text-from-image\n",
					"myimage2.txt":  "text-from-image\n",
				}
				for filename, expectedText := range expected {
					actualText := h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", filename))
					h.AssertEq(t, actualText, expectedText)
				}

				// Final Image should have same number of layers as initial image
				numLayers := h.Run(t, exec.Command("docker", "inspect", repoName, "-f", "{{len .RootFS.Layers}}"))
				h.AssertEq(t, numLayers, origNumLayers)
			})
		})
	})

	when("#TopLayer", func() {
		when("image exists", func() {
			var expectedTopLayer string
			it.Before(func() {
				expectedTopLayer = createImageOnLocal(t, repoName, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)
			})

			it.After(func() {
				h.Run(t, exec.Command("docker", "rmi", repoName))
			})

			it("returns the digest for the top layer (useful for rebasing)", func() {
				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)

				actualTopLayer, err := img.TopLayer()
				h.AssertNil(t, err)

				h.AssertEq(t, actualTopLayer, expectedTopLayer)
			})
		})
	})

	when("#AddLayer", func() {
		var (
			tarPath string
			img     image.Image
			origID  string
		)
		it.Before(func() {
			createImageOnLocal(t, repoName, `
					FROM busybox
					RUN echo -n old-layer > old-layer.txt
				`)
			tr, err := (&fs.FS{}).CreateSingleFileTar("/new-layer.txt", "new-layer")
			h.AssertNil(t, err)
			tarFile, err := ioutil.TempFile("", "add-layer-test")
			h.AssertNil(t, err)
			defer tarFile.Close()
			_, err = io.Copy(tarFile, tr)
			h.AssertNil(t, err)
			tarPath = tarFile.Name()

			img, err = factory.NewLocal(repoName, false)
			h.AssertNil(t, err)
			origID = h.ImageID(t, repoName)
		})

		it.After(func() {
			err := os.Remove(tarPath)
			h.AssertNil(t, err)
			h.Run(t, exec.Command("docker", "rmi", repoName))
			h.Run(t, exec.Command("docker", "rmi", origID))
		})

		it("appends a layer", func() {
			err := img.AddLayer(tarPath)
			h.AssertNil(t, err)

			_, err = img.Save()
			h.AssertNil(t, err)

			output := h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "/old-layer.txt"))
			h.AssertEq(t, output, "old-layer")

			output = h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "/new-layer.txt"))
			h.AssertEq(t, output, "new-layer")
		})
	})

	when("#ReuseLayer", func() {
		var (
			layer1SHA string
			layer2SHA string
			img       image.Image
			origID    string
		)
		it.Before(func() {
			var err error

			createImageOnLocal(t, repoName, fmt.Sprintf(`
					FROM busybox
					LABEL repo_name_for_randomisation=%s
					RUN echo -n old-layer-1 > layer-1.txt
					RUN echo -n old-layer-2 > layer-2.txt
				`, repoName))

			layer1SHA = strings.TrimSpace(h.Run(t, exec.Command("docker", "inspect", repoName, "-f", "{{index .RootFS.Layers 1}}")))
			layer2SHA = strings.TrimSpace(h.Run(t, exec.Command("docker", "inspect", repoName, "-f", "{{index .RootFS.Layers 2}}")))
			img, err = factory.NewLocal("busybox", false)
			h.AssertNil(t, err)

			img.Rename(repoName)
			h.AssertNil(t, err)
			origID = h.ImageID(t, repoName)
		})

		it.After(func() {
			h.Run(t, exec.Command("docker", "rmi", repoName))
			h.Run(t, exec.Command("docker", "rmi", origID))
		})

		it("reuses a layer", func() {
			err := img.ReuseLayer(layer2SHA)
			h.AssertNil(t, err)

			_, err = img.Save()
			h.AssertNil(t, err)

			output := h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "/layer-2.txt"))
			h.AssertEq(t, output, "old-layer-2")

			// Confirm layer-1.txt does not exist
			_, err = h.RunE(exec.Command("docker", "run", "--rm", repoName, "cat", "/layer-1.txt"))
			h.AssertContains(t, err.Error(), "cat: can't open '/layer-1.txt': No such file or directory")
		})

		it("does not download the old image if layers are directly above (performance)", func() {
			err := img.ReuseLayer(layer1SHA)
			h.AssertNil(t, err)

			_, err = img.Save()
			h.AssertNil(t, err)

			output := h.Run(t, exec.Command("docker", "run", "--rm", repoName, "cat", "/layer-1.txt"))
			h.AssertEq(t, output, "old-layer-1")

			// Confirm layer-2.txt does not exist
			_, err = h.RunE(exec.Command("docker", "run", "--rm", repoName, "cat", "/layer-2.txt"))
			h.AssertContains(t, err.Error(), "cat: can't open '/layer-2.txt': No such file or directory")
		})
	})

	when("#Save", func() {
		var (
			img    image.Image
			origID string
		)
		when("image exists", func() {
			it.Before(func() {
				var err error
				createImageOnLocal(t, repoName, `
					FROM busybox
					LABEL mykey=oldValue
				`)
				img, err = factory.NewLocal(repoName, false)
				h.AssertNil(t, err)
				origID = h.ImageID(t, repoName)
			})

			it.After(func() {
				h.Run(t, exec.Command("docker", "rmi", repoName))
				h.Run(t, exec.Command("docker", "rmi", origID))
			})

			it("returns the image digest", func() {
				err := img.SetLabel("mykey", "newValue")
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
