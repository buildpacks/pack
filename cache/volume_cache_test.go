package cache_test

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/cache"
	h "github.com/buildpack/pack/testhelpers"
)

func TestVolumeCache(t *testing.T) {
	h.RequireDocker(t)
	color.Disable(true)
	defer func() { color.Disable(false) }()
	rand.Seed(time.Now().UTC().UnixNano())

	spec.Run(t, "VolumeCache", testCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCache(t *testing.T, when spec.G, it spec.S) {
	when("#NewVolumeCache", func() {
		var dockerClient *client.Client

		it.Before(func() {
			var err error
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
		})

		it("adds suffix to calculated name", func() {
			ref, err := name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			if !strings.HasSuffix(subject.Name(), ".some-suffix") {
				t.Fatalf("Calculated volume name '%s' should end with '.some-suffix'", subject.Name())
			}
		})

		it("reusing the same cache for the same repo name", func() {
			ref, err := name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			expected := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			if subject.Name() != expected.Name() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("supplies different volumes for different tags", func() {
			ref, err := name.ParseReference("my/repo:other-tag", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			notExpected := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			if subject.Name() == notExpected.Name() {
				t.Fatalf("Different image tags should result in different volumes")
			}
		})

		it("supplies different volumes for different registries", func() {
			ref, err := name.ParseReference("registry.com/my/repo:other-tag", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			notExpected := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			if subject.Name() == notExpected.Name() {
				t.Fatalf("Different image registries should result in different volumes")
			}
		})

		it("resolves implied tag", func() {
			ref, err := name.ParseReference("my/repo:latest", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			expected := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			if subject.Name() != expected.Name() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("resolves implied registry", func() {
			ref, err := name.ParseReference("index.docker.io/my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			ref, err = name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			expected := cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			if subject.Name() != expected.Name() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})
	})

	when("#Clear", func() {
		var (
			volumeName   string
			dockerClient *client.Client
			subject      *cache.VolumeCache
			ctx          context.Context
		)

		it.Before(func() {
			var err error
			dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
			h.AssertNil(t, err)
			ctx = context.TODO()

			ref, err := name.ParseReference(h.RandString(10), name.WeakValidation)
			h.AssertNil(t, err)
			subject = cache.NewVolumeCache(ref, "some-suffix", dockerClient)
			h.AssertNil(t, err)
			volumeName = subject.Name()
		})

		when("there is a cache volume", func() {
			it.Before(func() {
				dockerClient.VolumeCreate(context.TODO(), volume.VolumeCreateBody{
					Name: volumeName,
				})
			})

			it("removes the volume", func() {
				err := subject.Clear(ctx)
				h.AssertNil(t, err)
				volumes, err := dockerClient.VolumeList(context.TODO(), filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: volumeName,
				}))
				h.AssertNil(t, err)
				h.AssertEq(t, len(volumes.Volumes), 0)
			})
		})

		when("there is no cache volume", func() {
			it("does not fail", func() {
				err := subject.Clear(ctx)
				h.AssertNil(t, err)
			})
		})
	})
}
