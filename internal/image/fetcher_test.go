package image_test

import (
	"context"
	"fmt"
	"io/ioutil"
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

	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/logging"
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
		fetcher  *image.Fetcher
		repoName string
		repo     string
	)

	it.Before(func() {
		repo = "some-org/" + h.RandString(10)
		repoName = registryConfig.RepoName(repo)
		fetcher = image.NewFetcher(logging.NewLogWithWriters(ioutil.Discard, ioutil.Discard), docker)
	})

	when("#Fetch", func() {
		when("daemon is false", func() {
			when("there is a remote image", func() {
				it.Before(func() {
					img, err := remote.NewImage(repoName, authn.DefaultKeychain)
					h.AssertNil(t, err)

					h.AssertNil(t, img.Save())
				})

				it("returns the remote image", func() {
					_, err := fetcher.Fetch(context.TODO(), repoName, false, false)
					h.AssertNil(t, err)
				})
			})

			when("there is no remote image", func() {
				it("returns an error", func() {
					_, err := fetcher.Fetch(context.TODO(), repoName, false, false)
					h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist in registry", repoName))
				})
			})
		})

		when("daemon is true", func() {
			when("pull is false", func() {
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
						_, err := fetcher.Fetch(context.TODO(), repoName, true, false)
						h.AssertNil(t, err)
					})
				})

				when("there is no local image", func() {
					it("returns an error", func() {
						_, err := fetcher.Fetch(context.TODO(), repoName, true, false)
						h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
					})
				})
			})

			when("pull is true", func() {
				when("there is a remote image", func() {
					it.Before(func() {
						// Instantiate a pull-able local image
						// as opposed to a remote image so that the image
						// is created with the OS of the docker daemon
						img, err := local.NewImage(repoName, docker)
						h.AssertNil(t, err)
						defer h.DockerRmi(docker, repoName)

						h.AssertNil(t, img.Save())

						h.AssertNil(t, h.PushImage(docker, img.Name(), registryConfig))
					})

					it.After(func() {
						h.DockerRmi(docker, repoName)
					})

					it("pull the image and return the local copy", func() {
						_, err := fetcher.Fetch(context.TODO(), repoName, true, true)
						h.AssertNil(t, err)
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
							_, err := fetcher.Fetch(context.TODO(), repoName, true, true)
							h.AssertNil(t, err)
						})
					})

					when("there is no local image", func() {
						it("returns an error", func() {
							_, err := fetcher.Fetch(context.TODO(), repoName, true, true)
							h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
						})
					})
				})
			})
		})
	})
}
