package image_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/pkg/image"
	h "github.com/buildpacks/pack/testhelpers"
)

var docker client.CommonAPIClient
var registryConfig *h.TestRegistryConfig

func TestFetcher(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.Disable(true)
	defer color.Disable(false)

	h.RequireDocker(t)

	registryConfig = h.RunRegistry(t)
	defer registryConfig.StopRegistry(t)

	// TODO: is there a better solution to the auth problem?
	os.Setenv("DOCKER_CONFIG", registryConfig.DockerConfigDir)

	var err error
	docker, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)
	spec.Run(t, "Fetcher", testFetcher, spec.Report(report.Terminal{}))
}

func testFetcher(t *testing.T, when spec.G, it spec.S) {
	var (
		imageFetcher *image.Fetcher
		repoName     string
		repo         string
		outBuf       bytes.Buffer
	)

	it.Before(func() {
		repo = "some-org/" + h.RandString(10)
		repoName = registryConfig.RepoName(repo)
		imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker)
	})

	when("#Fetch", func() {
		when("daemon is false", func() {
			when("PullAlways", func() {
				when("there is a remote image", func() {
					it.Before(func() {
						img, err := remote.NewImage(repoName, authn.DefaultKeychain)
						h.AssertNil(t, err)

						h.AssertNil(t, img.Save())
					})

					it("returns the remote image", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: false, PullPolicy: image.PullAlways})
						h.AssertNil(t, err)
					})
				})

				when("there is no remote image", func() {
					it("returns an error", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: false, PullPolicy: image.PullAlways})
						h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist in registry", repoName))
					})
				})
			})

			when("PullIfNotPresent", func() {
				when("there is a remote image", func() {
					it.Before(func() {
						img, err := remote.NewImage(repoName, authn.DefaultKeychain)
						h.AssertNil(t, err)

						h.AssertNil(t, img.Save())
					})

					it("returns the remote image", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: false, PullPolicy: image.PullIfNotPresent})
						h.AssertNil(t, err)
					})
				})

				when("there is no remote image", func() {
					it("returns an error", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: false, PullPolicy: image.PullIfNotPresent})
						h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist in registry", repoName))
					})
				})
			})
		})

		when("daemon is true", func() {
			when("PullNever", func() {
				when("there is a local image", func() {
					it.Before(func() {
						// Make sure the repoName is not a valid remote repo.
						// This is to verify that no remote check is made
						// when there's a valid local image.
						repoName = "invalidhost" + repoName

						img, err := local.NewImage(repoName, docker)
						h.AssertNil(t, err)

						h.AssertNil(t, img.Save())
					})

					it.After(func() {
						h.DockerRmi(docker, repoName)
					})

					it("returns the local image", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullNever})
						h.AssertNil(t, err)
					})
				})

				when("there is no local image", func() {
					it("returns an error", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullNever})
						h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
					})
				})
			})

			when("PullAlways", func() {
				when("there is a remote image", func() {
					var (
						logger *logging.LogWithWriters
						output func() string
					)

					it.Before(func() {
						// Instantiate a pull-able local image
						// as opposed to a remote image so that the image
						// is created with the OS of the docker daemon
						img, err := local.NewImage(repoName, docker)
						h.AssertNil(t, err)
						defer h.DockerRmi(docker, repoName)

						h.AssertNil(t, img.Save())

						h.AssertNil(t, h.PushImage(docker, img.Name(), registryConfig))

						var outCons *color.Console
						outCons, output = h.MockWriterAndOutput()
						logger = logging.NewLogWithWriters(outCons, outCons)
						imageFetcher = image.NewFetcher(logger, docker)
					})

					it.After(func() {
						h.DockerRmi(docker, repoName)
					})

					it("pull the image and return the local copy", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways})
						h.AssertNil(t, err)
						h.AssertNotEq(t, output(), "")
					})

					it("doesn't log anything in quiet mode", func() {
						logger.WantQuiet(true)
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways})
						h.AssertNil(t, err)
						h.AssertEq(t, output(), "")
					})
				})

				when("there is no remote image", func() {
					when("there is a local image", func() {
						it.Before(func() {
							img, err := local.NewImage(repoName, docker)
							h.AssertNil(t, err)

							h.AssertNil(t, img.Save())
						})

						it.After(func() {
							h.DockerRmi(docker, repoName)
						})

						it("returns the local image", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways})
							h.AssertNil(t, err)
						})
					})

					when("there is no local image", func() {
						it("returns an error", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways})
							h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
						})
					})
				})

				when("image platform is specified", func() {
					it("passes the platform argument to the daemon", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways, Platform: "some-unsupported-platform"})
						h.AssertError(t, err, "unknown operating system or architecture")
					})
				})
			})

			when("PullIfNotPresent", func() {
				when("there is a remote image", func() {
					var (
						label          = "label"
						remoteImgLabel string
					)

					it.Before(func() {
						// Instantiate a pull-able local image
						// as opposed to a remote image so that the image
						// is created with the OS of the docker daemon
						remoteImg, err := local.NewImage(repoName, docker)
						h.AssertNil(t, err)
						defer h.DockerRmi(docker, repoName)

						h.AssertNil(t, remoteImg.SetLabel(label, "1"))
						h.AssertNil(t, remoteImg.Save())

						h.AssertNil(t, h.PushImage(docker, remoteImg.Name(), registryConfig))

						remoteImgLabel, err = remoteImg.Label(label)
						h.AssertNil(t, err)
					})

					it.After(func() {
						h.DockerRmi(docker, repoName)
					})

					when("there is a local image", func() {
						var localImgLabel string

						it.Before(func() {
							localImg, err := local.NewImage(repoName, docker)
							h.AssertNil(t, localImg.SetLabel(label, "2"))
							h.AssertNil(t, err)

							h.AssertNil(t, localImg.Save())

							localImgLabel, err = localImg.Label(label)
							h.AssertNil(t, err)
						})

						it.After(func() {
							h.DockerRmi(docker, repoName)
						})

						it("returns the local image", func() {
							fetchedImg, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertNil(t, err)
							h.AssertNotContains(t, outBuf.String(), "Pulling image")

							fetchedImgLabel, err := fetchedImg.Label(label)
							h.AssertNil(t, err)

							h.AssertEq(t, fetchedImgLabel, localImgLabel)
							h.AssertNotEq(t, fetchedImgLabel, remoteImgLabel)
						})
					})

					when("there is no local image", func() {
						it("returns the remote image", func() {
							fetchedImg, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertNil(t, err)

							fetchedImgLabel, err := fetchedImg.Label(label)
							h.AssertNil(t, err)
							h.AssertEq(t, fetchedImgLabel, remoteImgLabel)
						})
					})
				})

				when("there is no remote image", func() {
					when("there is a local image", func() {
						it.Before(func() {
							img, err := local.NewImage(repoName, docker)
							h.AssertNil(t, err)

							h.AssertNil(t, img.Save())
						})

						it.After(func() {
							h.DockerRmi(docker, repoName)
						})

						it("returns the local image", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertNil(t, err)
						})
					})

					when("there is no local image", func() {
						it("returns an error", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
						})
					})
				})

				when("image platform is specified", func() {
					it("passes the platform argument to the daemon", func() {
						_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent, Platform: "some-unsupported-platform"})
						h.AssertError(t, err, "unknown operating system or architecture")
					})
				})
			})
		})
	})
}
