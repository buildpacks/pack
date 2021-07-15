package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/api"
	"github.com/docker/docker/api/types"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/blob"
	cfg "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	image "github.com/buildpacks/pack/internal/image"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestBuildpackDownloader(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "BuildpackDownloader", testBuildpackDownloader, spec.Parallel(), spec.Report(report.Terminal{}))
}
func testBuildpackDownloader(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController      *gomock.Controller
		mockDownloader      *testmocks.MockDownloader
		mockImageFactory    *testmocks.MockImageFactory
		mockImageFetcher    *testmocks.MockImageFetcher
		mockDockerClient    *testmocks.MockCommonAPIClient
		subject             *pack.Client
		buildpackDownloader pack.BuildpackDownloader
		logger              logging.Logger
		out                 bytes.Buffer
		tmpDir              string
	)
	var createBuildpack = func(descriptor dist.BuildpackDescriptor) string {
		bp, err := ifakes.NewFakeBuildpackBlob(descriptor, 0644)
		h.AssertNil(t, err)
		url := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
		mockDownloader.EXPECT().Download(gomock.Any(), url).Return(bp, nil).AnyTimes()
		return url
	}

	var createPackage = func(imageName string) *fakes.Image {
		packageImage := fakes.NewImage(imageName, "", nil)
		mockImageFactory.EXPECT().NewImage(packageImage.Name(), false, "linux").Return(packageImage, nil)

		h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
			Name: packageImage.Name(),
			Config: pubbldpkg.Config{
				Platform: dist.Platform{OS: "linux"},
				Buildpack: dist.BuildpackURI{URI: createBuildpack(dist.BuildpackDescriptor{
					API:    api.MustParse("0.3"),
					Info:   dist.BuildpackInfo{ID: "example/foo", Version: "1.1.0"},
					Stacks: []dist.Stack{{ID: "some.stack.id"}},
				})},
			},
			Publish: true,
		}))

		return packageImage
	}
	it.Before(func() {
		logger = ilogging.NewLogWithWriters(&out, &out, ilogging.WithVerbose())
		mockController = gomock.NewController(t)
		mockDownloader = testmocks.NewMockDownloader(mockController)
		mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
		mockImageFactory = testmocks.NewMockImageFactory(mockController)
		mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)
		mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
		mockDownloader.EXPECT().Download(gomock.Any(), "some/buildpack/dir").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
		var err error
		subject, err = pack.NewClient(
			pack.WithLogger(logger),
			pack.WithDownloader(mockDownloader),
			pack.WithImageFactory(mockImageFactory),
			pack.WithFetcher(mockImageFetcher),
			pack.WithDockerClient(mockDockerClient),
		)
		h.AssertNil(t, err)

		buildpackDownloader = pack.NewBuildpackDownloader(logger, mockImageFetcher, mockDownloader)

		mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

		tmpDir, err = ioutil.TempDir("", "buildpack-downloader-test")
		h.AssertNil(t, err)
	})

	it.After(func() {
		mockController.Finish()
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#DownloadBuildpack", func() {
		var (
			packageImage *fakes.Image
		)

		shouldFetchPackageImageWith := func(demon bool, pull config.PullPolicy) {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), image.FetchOptions{Daemon: demon, PullPolicy: pull}).Return(packageImage, nil)
		}

		var buildpackDownloadOptions pack.BuildpackDownloadOptions = pack.BuildpackDownloadOptions{ImageOS: "linux"}
		when("package image lives in cnb registry", func() {
			var (
				registryFixture string
				packHome        string
				tmpDir          string
			)
			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "registry")
				h.AssertNil(t, err)

				packHome = filepath.Join(tmpDir, ".pack")
				err = os.MkdirAll(packHome, 0755)
				h.AssertNil(t, err)
				os.Setenv("PACK_HOME", packHome)

				registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))

				packageImage = createPackage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7")
			})
			it.After(func() {
				os.Unsetenv("PACK_HOME")
				err := os.RemoveAll(tmpDir)
				h.AssertNil(t, err)
			})
			when("daemon=true and pull-policy=always", func() {
				var configPath string

				it("should pull and use local package image", func() {
					packHome := filepath.Join(tmpDir, "packHome")
					h.AssertNil(t, os.Setenv("PACK_HOME", packHome))
					configPath = filepath.Join(packHome, "config.toml")
					h.AssertNil(t, cfg.Write(cfg.Config{
						Registries: []cfg.Registry{
							{
								Name: "some-registry",
								Type: "github",
								URL:  registryFixture,
							},
						},
					}, configPath))
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						RegistryName: "some-registry",
						ImageOS:      "linux",
						Daemon:       true,
						PullPolicy:   config.PullAlways,
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "urn:cnb:registry:example/foo@1.1.0", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})
			when("ambigious URI provided", func() {
				var configPath string

				it("should find package in registry", func() {
					packHome := filepath.Join(tmpDir, "packHome")
					h.AssertNil(t, os.Setenv("PACK_HOME", packHome))
					configPath = filepath.Join(packHome, "config.toml")
					h.AssertNil(t, cfg.Write(cfg.Config{
						Registries: []cfg.Registry{
							{
								Name: "some-registry",
								Type: "github",
								URL:  registryFixture,
							},
						},
					}, configPath))

					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						RegistryName: "some-registry",
						ImageOS:      "linux",
						Daemon:       true,
						PullPolicy:   config.PullAlways,
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "example/foo@1.1.0", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})
		})

		when("package image lives in docker registry", func() {
			it.Before(func() {
				packageImage = fakes.NewImage("docker.io/some/package-"+h.RandString(12), "", nil)
				mockImageFactory.EXPECT().NewImage(packageImage.Name(), false, "linux").Return(packageImage, nil)

				bpd := dist.BuildpackDescriptor{
					API:    api.MustParse("0.3"),
					Info:   dist.BuildpackInfo{ID: "some.pkg.bp", Version: "2.3.4", Homepage: "http://meta.buildpack"},
					Stacks: []dist.Stack{{ID: "some.stack.id"}},
				}

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: packageImage.Name(),
					Config: pubbldpkg.Config{
						Platform:  dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: createBuildpack(bpd)},
					},
					Publish:    true,
					PullPolicy: config.PullAlways,
				}))
			})

			prepareFetcherWithMissingPackageImage := func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), gomock.Any()).Return(nil, image.ErrNotFound)
			}

			when("image key is provided", func() {
				it("should succeed", func() {
					packageImage = createPackage("some/package:tag")
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						Daemon:     true,
						PullPolicy: config.PullAlways,
						ImageOS:    "linux",
						ImageName:  "some/package:tag",
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "example/foo")
				})
			})

			when("daemon=true and pull-policy=always", func() {
				it("should pull and use local package image", func() {
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     true,
						PullPolicy: config.PullAlways,
					}

					shouldFetchPackageImageWith(true, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "some.pkg.bp")
				})
			})

			when("daemon=false and pull-policy=always", func() {
				it("should use remote package image", func() {
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     false,
						PullPolicy: config.PullAlways,
					}

					shouldFetchPackageImageWith(false, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "some.pkg.bp")
				})
			})

			when("daemon=false and pull-policy=always", func() {
				it("should use remote package URI", func() {
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						ImageOS:    "linux",
						Daemon:     false,
						PullPolicy: config.PullAlways,
					}
					shouldFetchPackageImageWith(false, config.PullAlways)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), packageImage.Name(), buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "some.pkg.bp")
				})
			})

			when("publish=true and pull-policy=never", func() {
				it("should push to registry and not pull package image", func() {
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     false,
						PullPolicy: config.PullNever,
					}

					shouldFetchPackageImageWith(false, config.PullNever)
					mainBP, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertNil(t, err)
					h.AssertEq(t, mainBP.Descriptor().Info.ID, "some.pkg.bp")
				})
			})

			when("daemon=true pull-policy=never and there is no local package image", func() {
				it("should fail without trying to retrieve package image from registry", func() {
					buildpackDownloadOptions = pack.BuildpackDownloadOptions{
						ImageOS:    "linux",
						ImageName:  packageImage.Name(),
						Daemon:     true,
						PullPolicy: config.PullNever,
					}
					prepareFetcherWithMissingPackageImage()
					_, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
					h.AssertError(t, err, "not found")
				})
			})
		})
		when("package lives on filesystem", func() {
			it("should successfully retrieve package from absolute path", func() {
				buildpackPath := filepath.Join("testdata", "buildpack")
				buildpackURI, _ := paths.FilePathToURI(buildpackPath, "")
				mockDownloader.EXPECT().Download(gomock.Any(), buildpackURI).Return(blob.NewBlob(buildpackPath), nil).AnyTimes()
				mainBP, _, err := buildpackDownloader.Download(context.TODO(), buildpackURI, buildpackDownloadOptions)
				h.AssertNil(t, err)
				h.AssertEq(t, mainBP.Descriptor().Info.ID, "bp.one")
			})
			it("should successfully retrieve package from relative path", func() {
				buildpackPath := filepath.Join("testdata", "buildpack")
				buildpackURI, _ := paths.FilePathToURI(buildpackPath, "")
				mockDownloader.EXPECT().Download(gomock.Any(), buildpackURI).Return(blob.NewBlob(buildpackPath), nil).AnyTimes()
				buildpackDownloadOptions = pack.BuildpackDownloadOptions{
					ImageOS:         "linux",
					RelativeBaseDir: "testdata",
				}
				mainBP, _, err := buildpackDownloader.Download(context.TODO(), "buildpack", buildpackDownloadOptions)
				h.AssertNil(t, err)
				h.AssertEq(t, mainBP.Descriptor().Info.ID, "bp.one")
			})
		})
		when("package image is not a valid package", func() {
			it("should error", func() {
				notPackageImage := fakes.NewImage("docker.io/not/package", "", nil)

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), gomock.Any()).Return(notPackageImage, nil)
				h.AssertNil(t, notPackageImage.SetLabel("io.buildpacks.buildpack.layers", ""))

				buildpackDownloadOptions.ImageName = notPackageImage.Name()
				_, _, err := buildpackDownloader.Download(context.TODO(), "", buildpackDownloadOptions)
				h.AssertError(t, err, "extracting buildpacks from 'docker.io/not/package': could not find label 'io.buildpacks.buildpackage.metadata'")
			})
		})
		when("invalid buildpack URI", func() {
			when("buildpack URI is from=builder:fake", func() {
				it("errors", func() {
					_, _, err := subject.BuildpackDownloader.Download(context.TODO(), "from=builder:fake", buildpackDownloadOptions)
					h.AssertError(t, err, "'from=builder:fake' is not a valid identifier")
				})
			})

			when("buildpack URI is from=builder", func() {
				it("errors", func() {
					_, _, err := subject.BuildpackDownloader.Download(context.TODO(), "from=builder", buildpackDownloadOptions)
					h.AssertError(t, err,
						"invalid locator: FromBuilderLocator")
				})
			})

			when("buildpack URI is invalid registry", func() {
				it("errors", func() {
					buildpackDownloadOptions.RegistryName = "://bad-url"
					_, _, err := subject.BuildpackDownloader.Download(context.TODO(), "urn:cnb:registry:fake", buildpackDownloadOptions)
					h.AssertError(t, err,
						"invalid registry")
				})
			})

			when("buildpack is missing from registry", func() {
				var configPath string
				var registryFixture string

				it("errors", func() {
					registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))

					packHome := filepath.Join(tmpDir, "packHome")
					h.AssertNil(t, os.Setenv("PACK_HOME", packHome))

					configPath = filepath.Join(packHome, "config.toml")
					h.AssertNil(t, cfg.Write(cfg.Config{
						Registries: []cfg.Registry{
							{
								Name: "some-registry",
								Type: "github",
								URL:  registryFixture,
							},
						},
					}, configPath))

					buildpackDownloadOptions.RegistryName = "some-registry"
					_, _, err := subject.BuildpackDownloader.Download(context.TODO(), "urn:cnb:registry:fake", buildpackDownloadOptions)
					h.AssertError(t, err,
						"locating in registry")
				})
			})

			when("can't download image from registry", func() {
				var configPath string
				var registryFixture string

				it("errors", func() {
					registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))
					packHome := filepath.Join(tmpDir, "packHome")
					h.AssertNil(t, os.Setenv("PACK_HOME", packHome))

					configPath = filepath.Join(packHome, "config.toml")
					h.AssertNil(t, cfg.Write(cfg.Config{
						Registries: []cfg.Registry{
							{
								Name: "some-registry",
								Type: "github",
								URL:  registryFixture,
							},
						},
					}, configPath))

					packageImage := fakes.NewImage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", "", nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), image.FetchOptions{Daemon: false, PullPolicy: config.PullAlways}).Return(nil, errors.New("failed to pull"))

					buildpackDownloadOptions.RegistryName = "some-registry"
					_, _, err := subject.BuildpackDownloader.Download(context.TODO(), "urn:cnb:registry:example/foo@1.1.0", buildpackDownloadOptions)
					h.AssertError(t, err,
						"extracting from registry")
				})
			})
			when("buildpack URI is an invalid locator", func() {
				it("errors", func() {
					_, _, err := subject.BuildpackDownloader.Download(context.TODO(), "nonsense string here", buildpackDownloadOptions)
					h.AssertError(t, err,
						"invalid locator: InvalidLocator")
				})
			})
		})
	})
}
