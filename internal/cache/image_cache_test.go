package cache_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/cache"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestImageCache(t *testing.T) {
	h.RequireDocker(t)
	color.Disable(true)
	defer color.Disable(false)
	rand.Seed(time.Now().UTC().UnixNano())

	spec.Run(t, "ImageCache", testImageCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testImageCache(t *testing.T, when spec.G, it spec.S) {
	when("#NewImageCache", func() {
		var dockerClient client.CommonAPIClient

		it.Before(func() {
			var err error
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
		})

		it("reusing the same cache for the same repo name", func() {
			ref, err := name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewImageCache(ref, dockerClient)
			expected := cache.NewImageCache(ref, dockerClient)
			if subject.Name() != expected.Name() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("supplies different images for different tags", func() {
			ref, err := name.ParseReference("my/repo:other-tag", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewImageCache(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			notExpected := cache.NewImageCache(ref, dockerClient)
			if subject.Name() == notExpected.Name() {
				t.Fatalf("Different image tags should result in different images")
			}
		})

		it("supplies different images for different registries", func() {
			ref, err := name.ParseReference("registry.com/my/repo:other-tag", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewImageCache(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			notExpected := cache.NewImageCache(ref, dockerClient)
			if subject.Name() == notExpected.Name() {
				t.Fatalf("Different image registries should result in different images")
			}
		})

		it("resolves implied tag", func() {
			ref, err := name.ParseReference("my/repo:latest", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewImageCache(ref, dockerClient)

			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			expected := cache.NewImageCache(ref, dockerClient)

			h.AssertEq(t, subject.Name(), expected.Name())
		})

		it("resolves implied registry", func() {
			ref, err := name.ParseReference("index.docker.io/my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewImageCache(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			expected := cache.NewImageCache(ref, dockerClient)
			if subject.Name() != expected.Name() {
				t.Fatalf("The same repo name should result in the same image")
			}
		})
	})

	when("#Clear", func() {
		var (
			imageName    string
			dockerClient client.CommonAPIClient
			subject      *cache.ImageCache
			ctx          context.Context
		)

		it.Before(func() {
			var err error
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
			ctx = context.TODO()

			ref, err := name.ParseReference(h.RandString(10), name.WeakValidation)
			h.AssertNil(t, err)
			subject = cache.NewImageCache(ref, dockerClient)
			h.AssertNil(t, err)
			imageName = subject.Name()
		})

		when("there is a cache image", func() {
			it.Before(func() {
				h.CreateImage(t, dockerClient, imageName, fmt.Sprintf(`
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
