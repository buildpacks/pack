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

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/google/go-cmp/cmp"
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
		assertNil(t, err)
		factory = image.Factory{
			Docker: docker,
			Log:    log.New(&buf, "", log.LstdFlags),
			Stdout: &buf,
			FS:     &fs.FS{},
		}
		repoName = "pack-image-test-" + randString(10)
	})
	it.After(func() {
		// exec.Command("docker", "rmi", "-f", repoName).Run()
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
			})
			it("returns the label value", func() {
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "myvalue")
			})

			it("returns an empty string for a missing label", func() {
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				label, err := img.Label("missing-label")
				assertNil(t, err)
				assertEq(t, label, "")
			})
		})

		when("image NOT exists", func() {
			it("returns an error", func() {
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				_, err = img.Label("mykey")
				assertError(t, err, fmt.Sprintf("failed to get label, image '%s' does not exist", repoName))
			})
		})
	})

	when("#Name", func() {
		it("always returns the original name", func() {
			img, _ := factory.NewLocal(repoName, false)
			assertEq(t, img.Name(), repoName)
		})
	})

	when("#Digest", func() {
		when("image exists", func() {
			var expectedDigest string
			it.Before(func() {
				var buf bytes.Buffer
				cmd := exec.Command("docker", "pull", "busybox:1.29")
				cmd.Stdout = &buf
				assertNil(t, cmd.Run())
				regex := regexp.MustCompile(`Digest: (sha256:\w*)`)
				matches := regex.FindStringSubmatch(buf.String())
				if len(matches) < 2 {
					t.Fatalf("digest regexp failed")
				}
				expectedDigest = matches[1]
			})

			it.After(func() {
				cmd := exec.Command("docker", "rmi", "busybox:1.29")
				assertNil(t, cmd.Run())
			})

			it("returns the image digest", func() {
				img, _ := factory.NewLocal("busybox:1.29", true)
				digest, err := img.Digest()
				assertNil(t, err)
				assertEq(t, digest, expectedDigest)
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
				assertNil(t, cmd.Run())
			})
			it("sets label on img object", func() {
				img, _ := factory.NewLocal(repoName, false)
				assertNil(t, img.SetLabel("mykey", "new-val"))
				label, err := img.Label("mykey")
				assertNil(t, err)
				assertEq(t, label, "new-val")
			})

			it("saves label to docker daemon", func() {
				img, _ := factory.NewLocal(repoName, false)
				assertNil(t, img.SetLabel("mykey", "new-val"))
				_, err := img.Save()
				assertNil(t, err)

				label, err := exec.Command("docker", "inspect", repoName, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "new-val")
			})
		})
	})

	when("#Rebase", func() {
		when("image exists", func() {
			var oldBase, oldTopLayer, newBase string
			it.Before(func() {
				oldBase = "pack-oldbase-test-" + randString(10)
				oldTopLayer = createImageOnLocal(t, oldBase, `
					FROM busybox
					RUN echo old-base > base.txt
					RUN echo text-old-base > otherfile.txt
				`)

				newBase = "pack-newbase-test-" + randString(10)
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
				exec.Command("docker", "rmi", oldBase, newBase).Run()
			})

			it("switches the base", func() {
				// Before
				txt, err := exec.Command("docker", "run", repoName, "cat", "base.txt").Output()
				assertNil(t, err)
				assertEq(t, string(txt), "old-base\n")

				// Run rebase
				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)
				newBaseImg, err := factory.NewLocal(newBase, false)
				assertNil(t, err)
				err = img.Rebase(oldTopLayer, newBaseImg)
				assertNil(t, err)
				_, err = img.Save()
				assertNil(t, err)

				// After
				txt, err = exec.Command("docker", "run", repoName, "cat", "base.txt").Output()
				assertNil(t, err)
				assertEq(t, string(txt), "new-base\n")
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
				createImageOnLocal(t, repoName, `
					FROM busybox
					LABEL mykey=oldValue
				`)

				img, err := factory.NewLocal(repoName, false)
				assertNil(t, err)

				err = img.SetLabel("mykey", "newValue")
				assertNil(t, err)

				imgDigest, err := img.Save()
				assertNil(t, err)

				label, err := exec.Command("docker", "inspect", imgDigest, "-f", `{{.Config.Labels.mykey}}`).Output()
				assertEq(t, strings.TrimSpace(string(label)), "newValue")
			})
		})
	})
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

// Assert deep equality (and provide useful difference as a test failure)
func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

func assertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}

func createImageOnLocal(t *testing.T, repoName, dockerFile string) string {
	t.Helper()

	cmd := exec.Command("docker", "build", "-t", repoName+":latest", "-")
	cmd.Stdin = strings.NewReader(dockerFile)
	run(t, cmd)

	topLayerJSON, err := exec.Command("docker", "inspect", repoName, "-f", `{{json .RootFS.Layers}}`).Output()
	assertNil(t, err)
	var layers []string
	assertNil(t, json.Unmarshal(topLayerJSON, &layers))
	topLayer := layers[len(layers)-1]

	return topLayer
}
