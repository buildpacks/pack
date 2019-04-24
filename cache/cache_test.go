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
	"github.com/fatih/color"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/cache"
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
		var dockerClient *client.Client

		it.Before(func() {
			var err error
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
		})

		it("reusing the same cache for the same repo name", func() {
			ref, err := name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.New(ref, dockerClient)
			expected := cache.New(ref, dockerClient)
			if subject.Image() != expected.Image() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("supplies different volumes for different tags", func() {
			ref, err := name.ParseReference("my/repo:other-tag", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.New(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			notExpected := cache.New(ref, dockerClient)
			if subject.Image() == notExpected.Image() {
				t.Fatalf("Different image tags should result in different volumes")
			}
		})

		it("supplies different volumes for different registries", func() {
			ref, err := name.ParseReference("registry.com/my/repo:other-tag", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.New(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			notExpected := cache.New(ref, dockerClient)
			if subject.Image() == notExpected.Image() {
				t.Fatalf("Different image registries should result in different volumes")
			}
		})

		it("resolves implied tag", func() {
			ref, err := name.ParseReference("my/repo:latest", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.New(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			expected := cache.New(ref, dockerClient)
			if subject.Image() != expected.Image() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("resolves implied registry", func() {
			ref, err := name.ParseReference("index.docker.io/my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.New(ref, dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			expected := cache.New(ref, dockerClient)
			if subject.Image() != expected.Image() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})
	})

	when("#Clear", func() {
		var (
			imageName    string
			dockerClient *client.Client
			subject      *cache.Cache
			ctx          context.Context
		)

		it.Before(func() {
			var err error
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
			ctx = context.TODO()

			ref, err := name.ParseReference(h.RandString(10), name.WeakValidation)
			h.AssertNil(t, err)
			subject = cache.New(ref, dockerClient)
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
