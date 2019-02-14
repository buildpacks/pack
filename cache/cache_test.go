package cache_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCache(t *testing.T) {
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
			if subject.Volume() != expected.Volume() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("supplies different volumes for different tags", func() {
			subject, err := cache.New("my/repo:other-tag", dockerClient)
			h.AssertNil(t, err)
			notExpected, _ := cache.New("my/repo", dockerClient)
			if subject.Volume() == notExpected.Volume() {
				t.Fatalf("Different image tags should result in different volumes")
			}
		})

		it("supplies different volumes for different registries", func() {
			subject, err := cache.New("registry.com/my/repo:other-tag", dockerClient)
			h.AssertNil(t, err)
			notExpected, _ := cache.New("my/repo", dockerClient)
			if subject.Volume() == notExpected.Volume() {
				t.Fatalf("Different image registries should result in different volumes")
			}
		})

		it("resolves implied tag", func() {
			subject, err := cache.New("my/repo:latest", dockerClient)
			h.AssertNil(t, err)
			expected, _ := cache.New("my/repo", dockerClient)
			if subject.Volume() != expected.Volume() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})

		it("resolves implied registry", func() {
			subject, err := cache.New("index.docker.io/my/repo", dockerClient)
			h.AssertNil(t, err)
			expected, _ := cache.New("my/repo", dockerClient)
			if subject.Volume() != expected.Volume() {
				t.Fatalf("The same repo name should result in the same volume")
			}
		})
	})

	when("#Clear", func() {
		var (
			volumeName   string
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
			volumeName = subject.Volume()
			h.AssertNil(t, err)
		})

		when("the volume is not attached to a container", func() {
			it.Before(func() {
				var err error
				_, err = dockerClient.VolumeCreate(context.TODO(), volume.VolumeCreateBody{Name: volumeName})
				h.AssertNil(t, err)
			})

			it.After(func() {
				err := dockerClient.VolumeRemove(context.TODO(), volumeName, true)
				h.AssertNil(t, err)
			})

			it("removes the volumes", func() {
				err := subject.Clear(ctx)
				h.AssertNil(t, err)
				body, err := dockerClient.VolumeList(context.TODO(), filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: volumeName,
				}))
				h.AssertNil(t, err)
				h.AssertEq(t, len(body.Volumes), 0)
			})
		})

		when("the volume is attached to a container", func() {
			it.Before(func() {
				o, err := dockerClient.ImagePull(context.TODO(), "busybox", types.ImagePullOptions{})
				h.AssertNil(t, err)
				_, err = io.Copy(ioutil.Discard, o)
				h.AssertNil(t, err)
			})

			when("container is created by pack", func() {
				var (
					containerBody container.ContainerCreateCreatedBody
					containerName string
				)
				it.Before(func() {
					var err error
					containerName = h.RandString(10)
					containerBody, err = dockerClient.ContainerCreate(context.TODO(), &container.Config{
						Image: "busybox",
						Labels: map[string]string{
							"author": "pack",
						},
					}, &container.HostConfig{
						Binds: []string{
							fmt.Sprintf("%s:%s:", volumeName, "/tmp"),
						},
					},
						nil,
						containerName)
					h.AssertNil(t, err)
				})

				it.After(func() {
					dockerClient.ContainerRemove(ctx, containerBody.ID, types.ContainerRemoveOptions{
						Force: true,
					})
				})

				when("the container is stopped", func() {
					it("removes the volumes and the container", func() {
						err := subject.Clear(ctx)
						h.AssertNil(t, err)

						body, err := dockerClient.VolumeList(context.TODO(), filters.NewArgs(filters.KeyValuePair{
							Key:   "name",
							Value: volumeName,
						}))
						h.AssertNil(t, err)
						h.AssertEq(t, len(body.Volumes), 0)
					})
				})

				when("the container is running", func() {
					it.Before(func() {
						dockerClient.ContainerStart(context.TODO(), containerBody.ID, types.ContainerStartOptions{})
					})

					it("removes the volumes and the container", func() {
						err := subject.Clear(ctx)
						h.AssertNil(t, err)

						body, err := dockerClient.VolumeList(context.TODO(), filters.NewArgs(filters.KeyValuePair{
							Key:   "name",
							Value: volumeName,
						}))
						h.AssertNil(t, err)
						h.AssertEq(t, len(body.Volumes), 0)
					})
				})
			})

			when("container is not created by pack", func() {
				var containerBody container.ContainerCreateCreatedBody
				it.Before(func() {
					var err error
					containerBody, err = dockerClient.ContainerCreate(context.TODO(), &container.Config{
						Image: "busybox",
					}, &container.HostConfig{
						Binds: []string{
							fmt.Sprintf("%s:%s:", volumeName, "/tmp"),
						},
					},
						nil,
						"some-container")
					h.AssertNil(t, err)
				})

				it.After(func() {
					err := dockerClient.ContainerRemove(context.TODO(), containerBody.ID, types.ContainerRemoveOptions{
						Force: true,
					})
					h.AssertNil(t, err)
				})

				it("does not removes the container or the volume", func() {
					err := subject.Clear(ctx)
					h.AssertError(t, err, fmt.Sprintf("volume in use by the container '%s' not created by pack", containerBody.ID))

					body, err := dockerClient.VolumeList(context.TODO(), filters.NewArgs(filters.KeyValuePair{
						Key:   "name",
						Value: volumeName,
					}))
					h.AssertNil(t, err)
					h.AssertEq(t, len(body.Volumes), 1)
				})
			})
		})
	})
}
