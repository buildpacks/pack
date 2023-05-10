package cache_test

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/buildpacks/pack/pkg/cache"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/daemon/names"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

func TestVolumeCache(t *testing.T) {
	h.RequireDocker(t)
	color.Disable(true)
	defer color.Disable(false)

	spec.Run(t, "VolumeCache", testCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCache(t *testing.T, when spec.G, it spec.S) {
	var dockerClient client.CommonAPIClient

	it.Before(func() {
		var err error
		dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
		h.AssertNil(t, err)
	})
	when("#NewVolumeCache", func() {
		when("volume cache name is empty", func() {
			it("adds suffix to calculated name", func() {
				ref, err := name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				if !strings.HasSuffix(subject.Name(), ".some-suffix") {
					t.Fatalf("Calculated volume name '%s' should end with '.some-suffix'", subject.Name())
				}
			})

			it("reusing the same cache for the same repo name", func() {
				ref, err := name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				expected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				if subject.Name() != expected.Name() {
					t.Fatalf("The same repo name should result in the same volume")
				}
			})

			it("supplies different volumes for different tags", func() {
				ref, err := name.ParseReference("my/repo:other-tag", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				notExpected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				if subject.Name() == notExpected.Name() {
					t.Fatalf("Different image tags should result in different volumes")
				}
			})

			it("supplies different volumes for different registries", func() {
				ref, err := name.ParseReference("registry.com/my/repo:other-tag", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				notExpected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				if subject.Name() == notExpected.Name() {
					t.Fatalf("Different image registries should result in different volumes")
				}
			})

			it("resolves implied tag", func() {
				ref, err := name.ParseReference("my/repo:latest", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				expected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				h.AssertEq(t, subject.Name(), expected.Name())
			})

			it("resolves implied registry", func() {
				ref, err := name.ParseReference("index.docker.io/my/repo", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				expected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				h.AssertEq(t, subject.Name(), expected.Name())
			})

			it("includes human readable information", func() {
				ref, err := name.ParseReference("myregistryhost:5000/fedora/httpd:version1.0", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				h.AssertContains(t, subject.Name(), "fedora_httpd_version1.0")
				h.AssertTrue(t, names.RestrictedNamePattern.MatchString(subject.Name()))
			})
		})

		when("volume cache name is not empty", func() {
			volumeName := "test-volume-name"
			cacheInfo := cache.CacheInfo{
				Format: cache.CacheVolume,
				Source: volumeName,
			}

			it("named volume created without suffix", func() {
				ref, err := name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cacheInfo, "some-suffix", dockerClient)

				if volumeName != subject.Name() {
					t.Fatalf("Volume name '%s' should be same as the name specified '%s'", subject.Name(), volumeName)
				}
			})

			it("reusing the same cache for the same repo name", func() {
				ref, err := name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cacheInfo, "some-suffix", dockerClient)

				expected := cache.NewVolumeCache(ref, cacheInfo, "some-suffix", dockerClient)
				if subject.Name() != expected.Name() {
					t.Fatalf("The same repo name should result in the same volume")
				}
			})

			it("supplies different volumes for different registries", func() {
				ref, err := name.ParseReference("registry.com/my/repo:other-tag", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				notExpected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				if subject.Name() == notExpected.Name() {
					t.Fatalf("Different image registries should result in different volumes")
				}
			})

			it("resolves implied tag", func() {
				ref, err := name.ParseReference("my/repo:latest", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				expected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				h.AssertEq(t, subject.Name(), expected.Name())
			})

			it("resolves implied registry", func() {
				ref, err := name.ParseReference("index.docker.io/my/repo", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				ref, err = name.ParseReference("my/repo", name.WeakValidation)
				h.AssertNil(t, err)
				expected := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
				h.AssertEq(t, subject.Name(), expected.Name())
			})

			it("includes human readable information", func() {
				ref, err := name.ParseReference("myregistryhost:5000/fedora/httpd:version1.0", name.WeakValidation)
				h.AssertNil(t, err)

				subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)

				h.AssertContains(t, subject.Name(), "fedora_httpd_version1.0")
				h.AssertTrue(t, names.RestrictedNamePattern.MatchString(subject.Name()))
			})
		})
	})

	when("#Clear", func() {
		var (
			volumeName   string
			dockerClient client.CommonAPIClient
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

			subject = cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
			volumeName = subject.Name()
		})

		when("there is a cache volume", func() {
			it.Before(func() {
				dockerClient.VolumeCreate(context.TODO(), volume.CreateOptions{
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

	when("#Type", func() {
		it("returns the cache type", func() {
			ref, err := name.ParseReference("my/repo", name.WeakValidation)
			h.AssertNil(t, err)
			subject := cache.NewVolumeCache(ref, cache.CacheInfo{}, "some-suffix", dockerClient)
			expected := cache.Volume
			h.AssertEq(t, subject.Type(), expected)
		})
	})
}
