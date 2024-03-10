package image_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	h "github.com/buildpacks/pack/testhelpers"
)

var docker client.CommonAPIClient
var logger logging.Logger
var registryConfig *h.TestRegistryConfig
var imageJSON *image.ImageJSON
var mockImagePullChecker = NewMockImagePullChecker(logger)

type MockPullChecker struct {
	*image.ImagePullPolicyManager
	MockCheckImagePullInterval func(imageID string, path string) (bool, error)
	MockRead                   func(path string) (*image.ImageJSON, error)
	MockPruneOldImages         func(f *image.Fetcher) error
	MockUpdateImagePullRecord  func(path string, imageID string, timestamp string) error
	MockWrite                  func(imageJSON *image.ImageJSON, path string) error
}

func NewMockImagePullChecker(logger logging.Logger) *MockPullChecker {
	return &MockPullChecker{
		ImagePullPolicyManager: image.NewPullPolicyManager(logger),
	}
}

func (m *MockPullChecker) CheckImagePullInterval(imageID string, path string) (bool, error) {
	if m.MockCheckImagePullInterval != nil {
		return m.MockCheckImagePullInterval(imageID, path)
	}
	return false, nil
}

func (m *MockPullChecker) Write(imageJSON *image.ImageJSON, path string) error {
	if m.MockWrite != nil {
		return m.MockWrite(imageJSON, path)
	}
	return nil
}

func (m *MockPullChecker) Read(path string) (*image.ImageJSON, error) {
	if m.MockRead != nil {
		return m.MockRead(path)
	}

	imageJSON = &image.ImageJSON{
		Interval: &image.Interval{
			PullingInterval: "7d",
			PruningInterval: "7d",
			LastPrune:       "2023-01-01T00:00:00Z",
		},
		Image: &image.ImageData{
			ImageIDtoTIME: map[string]string{
				"repoName": "2023-01-01T00:00:00Z",
			},
		},
	}

	return imageJSON, nil
}

func (m *MockPullChecker) PruneOldImages(f *image.Fetcher) error {
	if m.MockPruneOldImages != nil {
		return m.MockPruneOldImages(f)
	}

	return nil
}

func (m *MockPullChecker) UpdateImagePullRecord(path string, imageID string, timestamp string) error {
	if m.MockUpdateImagePullRecord != nil {
		fmt.Printf("checking wheather its calling or not")
		return m.MockUpdateImagePullRecord(path, imageID, timestamp)
	}

	return nil
}

func TestFetcher(t *testing.T) {
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
		osType       string
	)

	it.Before(func() {
		repo = "some-org/" + h.RandString(10)
		repoName = registryConfig.RepoName(repo)
		logger = logging.NewLogWithWriters(&outBuf, &outBuf)
		imageFetcher = image.NewFetcher(logger, docker, mockImagePullChecker)

		info, err := docker.Info(context.TODO())
		h.AssertNil(t, err)
		osType = info.OSType
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
						mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
							return true, nil
						}
						imageFetcher = image.NewFetcher(logger, docker, mockImagePullChecker)
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

					when("remote platform does not match", func() {
						it.Before(func() {
							img, err := remote.NewImage(repoName, authn.DefaultKeychain, remote.WithDefaultPlatform(imgutil.Platform{OS: osType, Architecture: ""}))
							h.AssertNil(t, err)
							h.AssertNil(t, img.Save())
						})

						it("retry without setting platform", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways, Platform: fmt.Sprintf("%s/%s", osType, runtime.GOARCH)})
							h.AssertNil(t, err)
						})
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

			when("PullWithInterval, PullHourly, PullDaily, PullWeekly", func() {
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

					when("there is no local image and CheckImagePullInterval returns true", func() {
						it.Before(func() {
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return true, nil
							}
							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
						})

						it("pulls the remote image and returns it", func() {
							fetchedImg, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullWeekly})
							h.AssertNil(t, err)

							fetchedImgLabel, err := fetchedImg.Label(label)
							h.AssertNil(t, err)
							h.AssertEq(t, fetchedImgLabel, remoteImgLabel)
						})
					})

					when("there is no local image and CheckImagePullInterval returns false", func() {
						it.Before(func() {
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return false, nil
							}

							imageJSON = &image.ImageJSON{
								Interval: &image.Interval{
									PullingInterval: "7d",
									PruningInterval: "7d",
									LastPrune:       "2023-01-01T00:00:00Z",
								},
								Image: &image.ImageData{
									ImageIDtoTIME: map[string]string{
										repoName: "2023-01-01T00:00:00Z",
									},
								},
							}

							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)

							mockImagePullChecker.MockRead = func(path string) (*image.ImageJSON, error) {
								return imageJSON, nil
							}
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
							mockImagePullChecker.MockRead = nil
						})

						it("returns an error and deletes the image record", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullWeekly})
							h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
							imageJSON, err = mockImagePullChecker.Read("")
							h.AssertNil(t, err)
							_, exists := imageJSON.Image.ImageIDtoTIME[repoName]
							h.AssertEq(t, exists, false)
						})
					})

					when("there is a local image and CheckImagePullInterval returns true", func() {
						it.Before(func() {
							img, err := local.NewImage(repoName, docker)
							h.AssertNil(t, err)

							h.AssertNil(t, img.Save())
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return true, nil
							}

							imageJSON = &image.ImageJSON{
								Interval: &image.Interval{
									PullingInterval: "7d",
									PruningInterval: "7d",
									LastPrune:       "2023-01-01T00:00:00Z",
								},
								Image: &image.ImageData{
									ImageIDtoTIME: map[string]string{
										repoName: "2023-01-01T00:00:00Z",
									},
								},
							}

							mockImagePullChecker.MockRead = func(path string) (*image.ImageJSON, error) {
								return imageJSON, nil
							}

							mockImagePullChecker.MockUpdateImagePullRecord = func(path string, imageID string, timestamp string) error {
								imageJSON, _ = mockImagePullChecker.Read("")
								imageJSON.Image.ImageIDtoTIME[repoName] = timestamp
								return nil
							}

							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
							mockImagePullChecker.MockRead = nil
							mockImagePullChecker.MockUpdateImagePullRecord = nil
							h.DockerRmi(docker, repoName)
						})

						it("pulls the remote image and returns it", func() {
							beforeFetch, _ := time.Parse(time.RFC3339, imageJSON.Image.ImageIDtoTIME[repoName])
							fmt.Printf("before fetch: %v\n", imageJSON.Image.ImageIDtoTIME[repoName])
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullWeekly})
							h.AssertNil(t, err)

							imageJSON, _ = mockImagePullChecker.Read("")

							afterFetch, _ := time.Parse(time.RFC3339, imageJSON.Image.ImageIDtoTIME[repoName])
							fmt.Printf("after fetch: %v\n", imageJSON.Image.ImageIDtoTIME[repoName])
							diff := beforeFetch.Before(afterFetch)
							h.AssertEq(t, diff, true)
						})
					})

					when("there is a local image and CheckImagePullInterval returns false", func() {
						it.Before(func() {
							localImg, err := local.NewImage(repoName, docker)
							h.AssertNil(t, err)
							h.AssertNil(t, localImg.SetLabel(label, "2"))

							h.AssertNil(t, localImg.Save())
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return true, nil
							}
							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
							h.DockerRmi(docker, repoName)
						})

						it("returns the local image", func() {
							fetchedImg, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertNil(t, err)

							fetchedImgLabel, err := fetchedImg.Label(label)
							h.AssertNil(t, err)
							h.AssertEq(t, fetchedImgLabel, "2")
						})
					})
				})

				when("there is no remote image", func() {
					var label string

					when("there is no local image and CheckImagePullInterval returns true", func() {
						it.Before(func() {
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return true, nil
							}
							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
						})

						it("try to pull the remote image and returns error", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullWeekly})
							h.AssertNotNil(t, err)
						})
					})

					when("there is no local image and CheckImagePullInterval returns false", func() {
						it.Before(func() {
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return false, nil
							}

							imageJSON = &image.ImageJSON{
								Interval: &image.Interval{
									PullingInterval: "7d",
									PruningInterval: "7d",
									LastPrune:       "2023-01-01T00:00:00Z",
								},
								Image: &image.ImageData{
									ImageIDtoTIME: map[string]string{
										repoName: "2023-01-01T00:00:00Z",
									},
								},
							}

							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
							mockImagePullChecker.MockRead = func(path string) (*image.ImageJSON, error) {
								return imageJSON, nil
							}
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
							mockImagePullChecker.MockRead = nil
						})

						it("returns an error and deletes the image record", func() {
							_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullWeekly})
							h.AssertError(t, err, fmt.Sprintf("image '%s' does not exist on the daemon", repoName))
							imageJSON, err = mockImagePullChecker.Read("")
							h.AssertNil(t, err)
							_, exists := imageJSON.Image.ImageIDtoTIME[repoName]
							h.AssertEq(t, exists, false)
						})
					})

					when("there is a local image and CheckImagePullInterval returns true", func() {
						it.Before(func() {
							localImg, err := local.NewImage(repoName, docker)
							h.AssertNil(t, err)
							h.AssertNil(t, localImg.SetLabel(label, "2"))

							h.AssertNil(t, localImg.Save())
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return true, nil
							}
							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
							h.DockerRmi(docker, repoName)
						})

						it("try to pull the remote image and returns local image", func() {
							fetchedImg, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertNil(t, err)

							fetchedImgLabel, err := fetchedImg.Label(label)
							h.AssertNil(t, err)
							h.AssertEq(t, fetchedImgLabel, "2")
						})
					})

					when("there is a local image and CheckImagePullInterval returns false", func() {
						it.Before(func() {
							localImg, err := local.NewImage(repoName, docker)
							h.AssertNil(t, err)
							h.AssertNil(t, localImg.SetLabel(label, "2"))

							h.AssertNil(t, localImg.Save())
							mockImagePullChecker.MockCheckImagePullInterval = func(imageID string, path string) (bool, error) {
								return true, nil
							}
							imageFetcher = image.NewFetcher(logging.NewLogWithWriters(&outBuf, &outBuf), docker, mockImagePullChecker)
						})

						it.After(func() {
							mockImagePullChecker.MockCheckImagePullInterval = nil
							h.DockerRmi(docker, repoName)
						})

						it("returns the local image", func() {
							fetchedImg, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{Daemon: true, PullPolicy: image.PullIfNotPresent})
							h.AssertNil(t, err)

							fetchedImgLabel, err := fetchedImg.Label(label)
							h.AssertNil(t, err)
							h.AssertEq(t, fetchedImgLabel, "2")
						})
					})
				})
			})
		})

		when("layout option is provided", func() {
			var (
				layoutOption image.LayoutOption
				imagePath    string
				tmpDir       string
				err          error
			)

			it.Before(func() {
				// set up local layout repo
				tmpDir, err = os.MkdirTemp("", "pack.fetcher.test")
				h.AssertNil(t, err)

				// dummy layer to validate sparse behavior
				tarDir := filepath.Join(tmpDir, "layer")
				err = os.MkdirAll(tarDir, os.ModePerm)
				h.AssertNil(t, err)
				layerPath := h.CreateTAR(t, tarDir, ".", -1)

				// set up the remote image to be used
				img, err := remote.NewImage(repoName, authn.DefaultKeychain)
				img.AddLayer(layerPath)
				h.AssertNil(t, err)
				h.AssertNil(t, img.Save())

				// set up layout options for the tests
				imagePath = filepath.Join(tmpDir, repo)
				layoutOption = image.LayoutOption{
					Path:   imagePath,
					Sparse: false,
				}
			})

			it.After(func() {
				err = os.RemoveAll(tmpDir)
				h.AssertNil(t, err)
			})

			when("sparse is false", func() {
				it("returns and layout image on disk", func() {
					_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{LayoutOption: layoutOption})
					h.AssertNil(t, err)

					// all layers were written
					h.AssertBlobsLen(t, imagePath, 3)
				})
			})

			when("sparse is true", func() {
				it("returns and layout image on disk", func() {
					layoutOption.Sparse = true
					_, err := imageFetcher.Fetch(context.TODO(), repoName, image.FetchOptions{LayoutOption: layoutOption})
					h.AssertNil(t, err)

					// only manifest and config was written
					h.AssertBlobsLen(t, imagePath, 2)
				})
			})
		})
	})
}
