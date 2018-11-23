package image_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	h "github.com/buildpack/pack/testhelpers"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestLocal(t *testing.T) {
	t.Parallel()
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "local", testLocal, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLocal(t *testing.T, when spec.G, it spec.S) {
	var factory image.Factory
	var buf bytes.Buffer
	var repoName string
	var dockerCli *docker.Client

	it.Before(func() {
		var err error
		dockerCli, err = docker.New()
		h.AssertNil(t, err)
		factory = image.Factory{
			Docker: dockerCli,
			Log:    log.New(&buf, "", log.LstdFlags),
			Stdout: &buf,
			FS:     &fs.FS{},
		}
		repoName = "pack-image-test-" + h.RandString(10)
	})

	when("#Label", func() {
		when("image exists", func() {
			it.Before(func() {
				h.CreateImageOnLocal(t, dockerCli, repoName, fmt.Sprintf(`
					FROM scratch
					LABEL repo_name_for_randomisation=%s
					LABEL mykey=myvalue other=data
				`, repoName))
			})

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
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
				h.AssertNil(t, dockerCli.PullImage("busybox:1.29"))
				expectedDigest = "sha256:2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812"
			})

			it("returns the image digest", func() {
				img, err := factory.NewLocal("busybox:1.29", true)
				h.AssertNil(t, err)
				digest, err := img.Digest()
				h.AssertNil(t, err)
				h.AssertEq(t, digest, expectedDigest)
			})
		})

		when("image exists but has no digest", func() {
			it.Before(func() {
				h.CreateImageOnLocal(t, dockerCli, repoName, `
					FROM scratch
					LABEL key=val
				`)
			})

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
			})

			it("returns an empty string", func() {
				img, err := factory.NewLocal(repoName, false)
				h.AssertNil(t, err)
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
				h.CreateImageOnLocal(t, dockerCli, repoName, `
					FROM scratch
					LABEL some-key=some-value
				`)
				img, err = factory.NewLocal(repoName, false)
				h.AssertNil(t, err)
				origID = h.ImageID(t, repoName)
			})

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName, origID))
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

				inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
				h.AssertNil(t, err)
				label = inspect.Config.Labels["somekey"]
				h.AssertEq(t, strings.TrimSpace(label), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase, origID string
			var origNumLayers int
			it.Before(func() {
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					newBase = "pack-newbase-test-" + h.RandString(10)
					h.CreateImageOnLocal(t, dockerCli, newBase, `
						FROM busybox
						RUN echo new-base > base.txt
						RUN echo text-new-base > otherfile.txt
					`)
				}()

				oldBase = "pack-oldbase-test-" + h.RandString(10)
				h.CreateImageOnLocal(t, dockerCli, oldBase, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)
				inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), oldBase)
				h.AssertNil(t, err)
				oldTopLayer = inspect.RootFS.Layers[len(inspect.RootFS.Layers)-1]

				h.CreateImageOnLocal(t, dockerCli, repoName, fmt.Sprintf(`
					FROM %s
					RUN echo text-from-image > myimage.txt
					RUN echo text-from-image > myimage2.txt
				`, oldBase))
				inspect, _, err = dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
				h.AssertNil(t, err)
				origNumLayers = len(inspect.RootFS.Layers)
				origID = inspect.ID

				wg.Wait()
			})

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName, oldBase, newBase, origID))
			})

			it("switches the base", func() {
				// Before
				txt, err := h.CopySingleFileFromImage(dockerCli, repoName, "base.txt")
				h.AssertNil(t, err)
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
				ctr, err := dockerCli.ContainerCreate(context.Background(), &container.Config{Image: repoName}, &container.HostConfig{}, nil, "")
				defer dockerCli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
				for filename, expectedText := range expected {
					actualText, err := h.CopySingleFileFromContainer(dockerCli, ctr.ID, filename)
					h.AssertNil(t, err)
					h.AssertEq(t, actualText, expectedText)
				}

				// Final Image should have same number of layers as initial image
				inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
				h.AssertNil(t, err)
				numLayers := len(inspect.RootFS.Layers)
				h.AssertEq(t, numLayers, origNumLayers)
			})
		})
	})

	when("#TopLayer", func() {
		when("image exists", func() {
			var expectedTopLayer string
			it.Before(func() {
				h.CreateImageOnLocal(t, dockerCli, repoName, `
				FROM busybox
				RUN echo old-base > base.txt
				RUN echo text-old-base > otherfile.txt
				`)

				inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
				h.AssertNil(t, err)
				expectedTopLayer = inspect.RootFS.Layers[len(inspect.RootFS.Layers)-1]
			})

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName))
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
			h.CreateImageOnLocal(t, dockerCli, repoName, `
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
			h.AssertNil(t, h.DockerRmi(dockerCli, repoName, origID))
		})

		it("appends a layer", func() {
			err := img.AddLayer(tarPath)
			h.AssertNil(t, err)

			_, err = img.Save()
			h.AssertNil(t, err)

			output, err := h.CopySingleFileFromImage(dockerCli, repoName, "old-layer.txt")
			h.AssertNil(t, err)
			h.AssertEq(t, output, "old-layer")

			output, err = h.CopySingleFileFromImage(dockerCli, repoName, "new-layer.txt")
			h.AssertNil(t, err)
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

			h.CreateImageOnLocal(t, dockerCli, repoName, fmt.Sprintf(`
					FROM busybox
					LABEL repo_name_for_randomisation=%s
					RUN echo -n old-layer-1 > layer-1.txt
					RUN echo -n old-layer-2 > layer-2.txt
				`, repoName))

			inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
			h.AssertNil(t, err)
			origID = inspect.ID

			layer1SHA = inspect.RootFS.Layers[1]
			layer2SHA = inspect.RootFS.Layers[2]

			img, err = factory.NewLocal("busybox", false)
			h.AssertNil(t, err)

			img.Rename(repoName)
			h.AssertNil(t, err)
		})

		it.After(func() {
			h.AssertNil(t, h.DockerRmi(dockerCli, repoName, origID))
		})

		it("reuses a layer", func() {
			err := img.ReuseLayer(layer2SHA)
			h.AssertNil(t, err)

			_, err = img.Save()
			h.AssertNil(t, err)

			output, err := h.CopySingleFileFromImage(dockerCli, repoName, "layer-2.txt")
			h.AssertNil(t, err)
			h.AssertEq(t, output, "old-layer-2")

			// Confirm layer-1.txt does not exist
			_, err = h.CopySingleFileFromImage(dockerCli, repoName, "layer-1.txt")
			h.AssertMatch(t, err.Error(), regexp.MustCompile(`Error: No such container:path: .*:layer-1.txt`))
		})

		it("does not download the old image if layers are directly above (performance)", func() {
			err := img.ReuseLayer(layer1SHA)
			h.AssertNil(t, err)

			_, err = img.Save()
			h.AssertNil(t, err)

			output, err := h.CopySingleFileFromImage(dockerCli, repoName, "layer-1.txt")
			h.AssertNil(t, err)
			h.AssertEq(t, output, "old-layer-1")

			// Confirm layer-2.txt does not exist
			_, err = h.CopySingleFileFromImage(dockerCli, repoName, "layer-2.txt")
			h.AssertMatch(t, err.Error(), regexp.MustCompile(`Error: No such container:path: .*:layer-2.txt`))
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
				h.CreateImageOnLocal(t, dockerCli, repoName, `
					FROM busybox
					LABEL mykey=oldValue
				`)
				img, err = factory.NewLocal(repoName, false)
				h.AssertNil(t, err)
				origID = h.ImageID(t, repoName)
			})

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, repoName, origID))
			})

			it("returns the image digest", func() {
				err := img.SetLabel("mykey", "newValue")
				h.AssertNil(t, err)

				imgDigest, err := img.Save()
				h.AssertNil(t, err)

				inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), imgDigest)
				h.AssertNil(t, err)
				label := inspect.Config.Labels["mykey"]
				h.AssertEq(t, strings.TrimSpace(label), "newValue")
			})
		})
	})
}
