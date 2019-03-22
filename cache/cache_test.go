package cache_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCache(t *testing.T) {
	h.RequireDocker(t)
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())

	spec.Run(t, "cache", testCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCache(t *testing.T, when spec.G, it spec.S) {
	when("#New", func() {
		var dockerClient *docker.Client
		it.Before(func() {
			var err error
			dockerClient, err = docker.New()
			h.AssertNil(t, err)
		})

		it("reusing the same cache for the same repo name", func() {
			subject, err := cache.New("my/repo", dockerClient)
			h.AssertNil(t, err)
			expected, _ := cache.New("my/repo", dockerClient)
			if subject.Image() != expected.Image() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("supplies different volumes for different tags", func() {
			subject, err := cache.New("my/repo:other-tag", dockerClient)
			h.AssertNil(t, err)
			notExpected, _ := cache.New("my/repo", dockerClient)
			if subject.Image() == notExpected.Image() {
				t.Fatalf("Different image tags should result in different volumes")
			}
		})

		it("supplies different volumes for different registries", func() {
			subject, err := cache.New("registry.com/my/repo:other-tag", dockerClient)
			h.AssertNil(t, err)
			notExpected, _ := cache.New("my/repo", dockerClient)
			if subject.Image() == notExpected.Image() {
				t.Fatalf("Different image registries should result in different volumes")
			}
		})

		it("resolves implied tag", func() {
			subject, err := cache.New("my/repo:latest", dockerClient)
			h.AssertNil(t, err)
			expected, _ := cache.New("my/repo", dockerClient)
			if subject.Image() != expected.Image() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("resolves implied registry", func() {
			subject, err := cache.New("index.docker.io/my/repo", dockerClient)
			h.AssertNil(t, err)
			expected, _ := cache.New("my/repo", dockerClient)
			if subject.Image() != expected.Image() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})
	})

	when("#Clear", func() {
		var (
			imageName    string
			dockerClient *docker.Client
			subject      *cache.Cache
			ctx          context.Context
		)

		it.Before(func() {
			var err error
			dockerClient, err = docker.New()
			h.AssertNil(t, err)
			ctx = context.TODO()

			subject, err = cache.New(h.RandString(10), dockerClient)
			h.AssertNil(t, err)
			imageName = subject.Image()
		})

		when("there is a cache image", func() {
			it.Before(func() {
				h.CreateImageOnLocal(t, dockerClient, imageName, fmt.Sprintf(`
FROM busybox
LABEL repo_name_for_randomisation=%s
`, imageName))
			})

			it("removes the image", func() {
				err := subject.Clear(ctx)
				h.AssertNil(t, err)
				images, err := dockerClient.ImageList(context.TODO(), types.ImageListOptions{
					Filters: filters.NewArgs(filters.KeyValuePair{
						Key:   "label",
						Value: "repo_name_for_randomisation=" + imageName,
					}),
				})
				h.AssertNil(t, err)
				h.AssertEq(t, len(images), 0)
			})
		})

		when("there is no cache image", func() {
			it("does not fail", func() {
				err := subject.Clear(ctx)
				h.AssertNil(t, err)
			})
		})
	})
}
