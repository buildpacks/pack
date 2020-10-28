package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
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
	pubbldr "github.com/buildpacks/pack/builder"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/builder"
	cfg "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/image"
	ilogging "github.com/buildpacks/pack/internal/logging"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/pack/testmocks"
)

func TestCreateBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			mockController     *gomock.Controller
			mockDownloader     *testmocks.MockDownloader
			mockImageFactory   *testmocks.MockImageFactory
			mockImageFetcher   *testmocks.MockImageFetcher
			mockDockerClient   *testmocks.MockCommonAPIClient
			fakeBuildImage     *fakes.Image
			fakeRunImage       *fakes.Image
			fakeRunImageMirror *fakes.Image
			opts               pack.CreateBuilderOptions
			subject            *pack.Client
			logger             logging.Logger
			out                bytes.Buffer
			tmpDir             string
		)

		it.Before(func() {
			logger = ilogging.NewLogWithWriters(&out, &out, ilogging.WithVerbose())
			mockController = gomock.NewController(t)
			mockDownloader = testmocks.NewMockDownloader(mockController)
			mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
			mockImageFactory = testmocks.NewMockImageFactory(mockController)
			mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)

			fakeBuildImage = fakes.NewImage("some/build-image", "", nil)
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "build:mixinY"]`))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_GROUP_ID", "4321"))

			fakeRunImage = fakes.NewImage("some/run-image", "", nil)
			h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			fakeRunImageMirror = fakes.NewImage("localhost:5000/some/run-image", "", nil)
			h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "some/buildpack/dir").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle", "platform-0.4")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle-platform-0-1").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle-platform-0.1")), nil).AnyTimes()

			var err error
			subject, err = pack.NewClient(
				pack.WithLogger(logger),
				pack.WithDownloader(mockDownloader),
				pack.WithImageFactory(mockImageFactory),
				pack.WithFetcher(mockImageFetcher),
				pack.WithDockerClient(mockDockerClient),
			)
			h.AssertNil(t, err)

			mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

			opts = pack.CreateBuilderOptions{
				BuilderName: "some/builder",
				Config: pubbldr.Config{
					Description: "Some description",
					Buildpacks: []pubbldr.BuildpackConfig{
						{
							BuildpackInfo: dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3", Homepage: "http://one.buildpack"},
							ImageOrURI: dist.ImageOrURI{
								BuildpackURI: dist.BuildpackURI{
									URI: "https://example.fake/bp-one.tgz",
								},
							},
						},
					},
					Order: []dist.OrderEntry{{
						Group: []dist.BuildpackRef{
							{BuildpackInfo: dist.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}, Optional: false},
						}},
					},
					Stack: pubbldr.StackConfig{
						ID:              "some.stack.id",
						BuildImage:      "some/build-image",
						RunImage:        "some/run-image",
						RunImageMirrors: []string{"localhost:5000/some/run-image"},
					},
					Lifecycle: pubbldr.LifecycleConfig{URI: "file:///some-lifecycle"},
				},
				Publish:    false,
				PullPolicy: config.PullAlways,
			}

			tmpDir, err = ioutil.TempDir("", "create-builder-test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		var prepareFetcherWithRunImages = func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", gomock.Any(), gomock.Any()).Return(fakeRunImage, nil).AnyTimes()
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", gomock.Any(), gomock.Any()).Return(fakeRunImageMirror, nil).AnyTimes()
		}

		var prepareFetcherWithBuildImage = func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", gomock.Any(), gomock.Any()).Return(fakeBuildImage, nil)
		}

		var successfullyCreateBuilder = func() *builder.Builder {
			t.Helper()

			err := subject.CreateBuilder(context.TODO(), opts)
			h.AssertNil(t, err)

			h.AssertEq(t, fakeBuildImage.IsSaved(), true)
			bldr, err := builder.FromImage(fakeBuildImage)
			h.AssertNil(t, err)

			return bldr
		}

		when("validating the builder config", func() {
			it("should fail when the stack ID is empty", func() {
				opts.Config.Stack.ID = ""

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack.id is required")
			})

			it("should fail when the stack ID from the builder config does not match the stack ID from the build image", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, config.PullAlways).Return(fakeBuildImage, nil)
				h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "other.stack.id"))
				prepareFetcherWithRunImages()

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from build image")
			})

			it("should fail when the build image is empty", func() {
				opts.Config.Stack.BuildImage = ""

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack.build-image is required")
			})

			it("should fail when the run image is empty", func() {
				opts.Config.Stack.RunImage = ""

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack.run-image is required")
			})

			it("should fail when lifecycle version is not a semver", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = ""
				opts.Config.Lifecycle.Version = "not-semver"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "'lifecycle.version' must be a valid semver")
			})

			it("should fail when both lifecycle version and uri are present", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = "file://some-lifecycle"
				opts.Config.Lifecycle.Version = "something"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "'lifecycle' can only declare 'version' or 'uri', not both")
			})

			it("should fail when buildpack ID does not match downloaded buildpack", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].ID = "does.not.match"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "buildpack from URI 'https://example.fake/bp-one.tgz' has ID 'bp.one' which does not match ID 'does.not.match' from builder config")
			})

			it("should fail when buildpack version does not match downloaded buildpack", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].Version = "0.0.0"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "buildpack from URI 'https://example.fake/bp-one.tgz' has version '1.2.3' which does not match version '0.0.0' from builder config")
			})
		})

		when("validating the run image config", func() {
			it("should fail when the stack ID from the builder config does not match the stack ID from the run image", func() {
				prepareFetcherWithRunImages()
				h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from run image 'some/run-image'")
			})

			it("should fail when the stack ID from the builder config does not match the stack ID from the run image mirrors", func() {
				prepareFetcherWithRunImages()
				h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from run image 'localhost:5000/some/run-image'")
			})

			it("should warn when the run image cannot be found", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, config.PullAlways).Return(fakeBuildImage, nil)

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", false, config.PullAlways).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", true, config.PullAlways).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", false, config.PullAlways).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", true, config.PullAlways).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)

				h.AssertContains(t, out.String(), "Warning: run image 'some/run-image' is not accessible")
			})

			it("should fail when not publish and the run image cannot be fetched", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", true, config.PullAlways).Return(nil, errors.New("yikes"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "failed to fetch image: yikes")
			})

			it("should fail when publish and the run image cannot be fetched", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", false, config.PullAlways).Return(nil, errors.New("yikes"))

				opts.Publish = true
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "failed to fetch image: yikes")
			})

			it("should fail when the run image isn't a valid image", func() {
				fakeImage := fakeBadImageStruct{}

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", gomock.Any(), gomock.Any()).Return(fakeImage, nil).AnyTimes()
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", gomock.Any(), gomock.Any()).Return(fakeImage, nil).AnyTimes()

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "failed to label image")
			})

			when("publish is true", func() {
				it("should only try to validate the remote run image", func() {
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, gomock.Any()).Times(0)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", true, gomock.Any()).Times(0)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", true, gomock.Any()).Times(0)

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", false, gomock.Any()).Return(fakeBuildImage, nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", false, gomock.Any()).Return(fakeRunImage, nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", false, gomock.Any()).Return(fakeRunImageMirror, nil)

					opts.Publish = true

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertNil(t, err)
				})
			})
		})

		when("creating the base builder", func() {
			when("build image not found", func() {
				it("should fail", func() {
					prepareFetcherWithRunImages()
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, config.PullAlways).Return(nil, image.ErrNotFound)

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "fetch build image: not found")
				})
			})

			when("build image isn't a valid image", func() {
				it("should fail", func() {
					fakeImage := fakeBadImageStruct{}

					prepareFetcherWithRunImages()
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, config.PullAlways).Return(fakeImage, nil)

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "failed to create builder: invalid build-image")
				})
			})

			when("windows containers", func() {
				when("experimental enabled", func() {
					it("succeeds", func() {
						packClientWithExperimental, err := pack.NewClient(
							pack.WithLogger(logger),
							pack.WithDownloader(mockDownloader),
							pack.WithImageFactory(mockImageFactory),
							pack.WithFetcher(mockImageFetcher),
							pack.WithExperimental(true),
						)
						h.AssertNil(t, err)

						prepareFetcherWithRunImages()

						h.AssertNil(t, fakeBuildImage.SetOS("windows"))
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, config.PullAlways).Return(fakeBuildImage, nil)

						err = packClientWithExperimental.CreateBuilder(context.TODO(), opts)
						h.AssertNil(t, err)
					})
				})

				when("experimental disabled", func() {
					it("fails", func() {
						prepareFetcherWithRunImages()

						h.AssertNil(t, fakeBuildImage.SetOS("windows"))
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", true, config.PullAlways).Return(fakeBuildImage, nil)

						err := subject.CreateBuilder(context.TODO(), opts)
						h.AssertError(t, err, "failed to create builder: Windows containers support is currently experimental.")
					})
				})
			})

			when("error downloading lifecycle", func() {
				it("should fail", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Lifecycle.URI = "fake"
					mockDownloader.EXPECT().Download(gomock.Any(), "fake").Return(nil, errors.New("error here")).AnyTimes()

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "downloading lifecycle")
				})
			})

			when("lifecycle isn't a valid lifecycle", func() {
				it("should fail", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Lifecycle.URI = "fake"
					mockDownloader.EXPECT().Download(gomock.Any(), "fake").Return(blob.NewBlob(filepath.Join("testdata", "empty-file")), nil).AnyTimes()

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "invalid lifecycle")
				})
			})
		})

		when("only lifecycle version is provided", func() {
			it("should download from predetermined uri", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = ""
				opts.Config.Lifecycle.Version = "3.4.5"

				mockDownloader.EXPECT().Download(
					gomock.Any(),
					"https://github.com/buildpacks/lifecycle/releases/download/v3.4.5/lifecycle-v3.4.5+linux.x86-64.tgz",
				).Return(
					blob.NewBlob(filepath.Join("testdata", "lifecycle", "platform-0.4")), nil,
				)

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})

			when("windows", func() {
				it("should download from predetermined uri", func() {
					packClientWithExperimental, err := pack.NewClient(
						pack.WithLogger(logger),
						pack.WithDownloader(mockDownloader),
						pack.WithImageFactory(mockImageFactory),
						pack.WithFetcher(mockImageFetcher),
						pack.WithExperimental(true),
					)
					h.AssertNil(t, err)

					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Lifecycle.URI = ""
					opts.Config.Lifecycle.Version = "3.4.5"
					h.AssertNil(t, fakeBuildImage.SetOS("windows"))

					mockDownloader.EXPECT().Download(
						gomock.Any(),
						"https://github.com/buildpacks/lifecycle/releases/download/v3.4.5/lifecycle-v3.4.5+windows.x86-64.tgz",
					).Return(
						blob.NewBlob(filepath.Join("testdata", "lifecycle", "platform-0.4")), nil,
					)

					err = packClientWithExperimental.CreateBuilder(context.TODO(), opts)
					h.AssertNil(t, err)
				})
			})
		})

		when("no lifecycle version or URI is provided", func() {
			it("should download default lifecycle", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Lifecycle.URI = ""
				opts.Config.Lifecycle.Version = ""

				mockDownloader.EXPECT().Download(
					gomock.Any(),
					fmt.Sprintf(
						"https://github.com/buildpacks/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz",
						builder.DefaultLifecycleVersion,
						builder.DefaultLifecycleVersion,
					),
				).Return(
					blob.NewBlob(filepath.Join("testdata", "lifecycle", "platform-0.4")), nil,
				)

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})

			when("windows", func() {
				it("should download default lifecycle", func() {
					packClientWithExperimental, err := pack.NewClient(
						pack.WithLogger(logger),
						pack.WithDownloader(mockDownloader),
						pack.WithImageFactory(mockImageFactory),
						pack.WithFetcher(mockImageFetcher),
						pack.WithExperimental(true),
					)
					h.AssertNil(t, err)

					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Lifecycle.URI = ""
					opts.Config.Lifecycle.Version = ""
					h.AssertNil(t, fakeBuildImage.SetOS("windows"))

					mockDownloader.EXPECT().Download(
						gomock.Any(),
						fmt.Sprintf(
							"https://github.com/buildpacks/lifecycle/releases/download/v%s/lifecycle-v%s+windows.x86-64.tgz",
							builder.DefaultLifecycleVersion,
							builder.DefaultLifecycleVersion,
						),
					).Return(
						blob.NewBlob(filepath.Join("testdata", "lifecycle", "platform-0.4")), nil,
					)

					err = packClientWithExperimental.CreateBuilder(context.TODO(), opts)
					h.AssertNil(t, err)
				})
			})
		})

		when("buildpack mixins are not satisfied", func() {
			it("should return an error", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.mixins", ""))

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "validating buildpacks: buildpack 'bp.one@1.2.3' requires missing mixin(s): build:mixinY, mixinX")
			})
		})

		when("creation succeeds", func() {
			it("should set basic metadata", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.Name(), "some/builder")
				h.AssertEq(t, bldr.Description(), "Some description")
				h.AssertEq(t, bldr.UID(), 1234)
				h.AssertEq(t, bldr.GID(), 4321)
				h.AssertEq(t, bldr.StackID, "some.stack.id")
			})

			it("should set buildpack and order metadata", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				bpInfo := dist.BuildpackInfo{
					ID:       "bp.one",
					Version:  "1.2.3",
					Homepage: "http://one.buildpack",
				}
				h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{bpInfo})
				bpInfo.Homepage = ""
				h.AssertEq(t, bldr.Order(), dist.Order{{
					Group: []dist.BuildpackRef{{
						BuildpackInfo: bpInfo,
						Optional:      false,
					}},
				}})
			})

			it("should embed the lifecycle", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				successfullyCreateBuilder()

				layerTar, err := fakeBuildImage.FindLayerWithPath("/cnb/lifecycle")
				h.AssertNil(t, err)
				h.AssertTarHasFile(t, layerTar, "/cnb/lifecycle/detector")
				h.AssertTarHasFile(t, layerTar, "/cnb/lifecycle/restorer")
				h.AssertTarHasFile(t, layerTar, "/cnb/lifecycle/analyzer")
				h.AssertTarHasFile(t, layerTar, "/cnb/lifecycle/builder")
				h.AssertTarHasFile(t, layerTar, "/cnb/lifecycle/exporter")
				h.AssertTarHasFile(t, layerTar, "/cnb/lifecycle/launcher")
			})

			it("should set lifecycle descriptor", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().Info.Version.String(), "0.0.0")
				//nolint:staticcheck
				h.AssertEq(t, bldr.LifecycleDescriptor().API.BuildpackVersion.String(), "0.2")
				//nolint:staticcheck
				h.AssertEq(t, bldr.LifecycleDescriptor().API.PlatformVersion.String(), "0.2")
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Deprecated.AsStrings(), []string{"0.2", "0.3"})
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Supported.AsStrings(), []string{"0.2", "0.3", "0.4"})
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Platform.Deprecated.AsStrings(), []string{"0.2"})
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Platform.Supported.AsStrings(), []string{"0.3", "0.4"})
			})

			it("should warn when deprecated Buildpack API version used", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Deprecated.AsStrings(), []string{"0.2", "0.3"})
				h.AssertContains(t, out.String(), fmt.Sprintf("Buildpack %s is using deprecated Buildpacks API version %s", style.Symbol("bp.one@1.2.3"), style.Symbol("0.3")))
			})

			it("shouldn't warn when Buildpack API version used isn't deprecated", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].URI = "https://example.fake/bp-one-with-api-4.tgz"
				mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one-with-api-4.tgz").Return(blob.NewBlob(filepath.Join("testdata", "buildpack-api-0.4")), nil).AnyTimes()
				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Deprecated.AsStrings(), []string{"0.2", "0.3"})
				h.AssertNotContains(t, out.String(), "is using deprecated Buildpacks API version")
			})
		})

		when("windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
			})

			it("disallows directory-based buildpacks", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].URI = "testdata/buildpack"

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t,
					err,
					"buildpack 'testdata/buildpack': directory-based buildpacks are not currently supported on Windows")
			})
		})

		when("is posix", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Skipped on windows")
			})

			it("supports directory buildpacks", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				directoryPath := "testdata/buildpack"
				opts.Config.Buildpacks[0].URI = directoryPath
				mockDownloader.EXPECT().Download(gomock.Any(), directoryPath).Return(blob.NewBlob(directoryPath), nil).AnyTimes()

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})

		when("invalid buildpack URI", func() {
			when("buildpack URI is from=builder:fake", func() {
				it("errors", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Buildpacks[0].URI = "from=builder:fake"

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err,
						"locator type from=builder:fake")
				})
			})

			when("buildpack URI is from=builder", func() {
				it("errors", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Buildpacks[0].URI = "from=builder"

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err,
						"invalid locator: FromBuilderLocator")
				})
			})

			when("buildpack URI is invalid registry", func() {
				it("errors", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Registry = "://bad-url"
					opts.Config.Buildpacks[0].URI = "urn:cnb:registry:fake"

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err,
						"invalid registry")
				})
			})

			when("buildpack is missing from registry", func() {
				var configPath string
				var registryFixture string

				it("errors", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()

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
					opts.Config.Buildpacks[0].URI = "urn:cnb:registry:fake"

					opts.Registry = "some-registry"

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err,
						"locating in registry")
				})
			})

			when("can't download image from registry", func() {
				var configPath string
				var registryFixture string

				it("errors", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()

					registryFixture = h.CreateRegistryFixture(t, tmpDir, filepath.Join("testdata", "registry"))
					opts.Config.Buildpacks[0].URI = "urn:cnb:registry:example/foo@1.1.0"

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

					opts.Registry = "some-registry"

					packageImage := fakes.NewImage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", "", nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), true, config.PullAlways).Return(nil, errors.New("failed to pull"))

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err,
						"extracting from registry")
				})
			})

			when("buildpack URI is an invalid locator", func() {
				it("errors", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Buildpacks[0].URI = "nonsense string here"

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err,
						"invalid locator: InvalidLocator")
				})
			})
		})

		when("package file", func() {
			it.Before(func() {
				cnbFile := filepath.Join(tmpDir, "bp_one1.cnb")
				buildpackPath := filepath.Join("testdata", "buildpack")
				mockDownloader.EXPECT().Download(gomock.Any(), buildpackPath).Return(blob.NewBlob(buildpackPath), nil)
				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
					Name: cnbFile,
					Config: pubbldpkg.Config{
						Buildpack: dist.BuildpackURI{URI: buildpackPath},
					},
					Format: "file",
				}))

				mockDownloader.EXPECT().Download(gomock.Any(), cnbFile).Return(blob.NewBlob(cnbFile), nil).AnyTimes()
				opts.Config.Buildpacks = []pubbldr.BuildpackConfig{{
					ImageOrURI: dist.ImageOrURI{BuildpackURI: dist.BuildpackURI{URI: cnbFile}},
				}}
			})

			it("package file is valid", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				bldr := successfullyCreateBuilder()

				bpInfo := dist.BuildpackInfo{
					ID:       "bp.one",
					Version:  "1.2.3",
					Homepage: "http://one.buildpack",
				}
				h.AssertEq(t, bldr.Buildpacks(), []dist.BuildpackInfo{bpInfo})
				bpInfo.Homepage = ""
				h.AssertEq(t, bldr.Order(), dist.Order{{
					Group: []dist.BuildpackRef{{
						BuildpackInfo: bpInfo,
						Optional:      false,
					}},
				}})
			})
		})

		when("packages", func() {
			var (
				packageImage *fakes.Image
			)

			createBuildpack := func(descriptor dist.BuildpackDescriptor) string {
				bp, err := ifakes.NewFakeBuildpackBlob(descriptor, 0644)
				h.AssertNil(t, err)
				url := fmt.Sprintf("https://example.com/bp.%s.tgz", h.RandString(12))
				mockDownloader.EXPECT().Download(gomock.Any(), url).Return(bp, nil).AnyTimes()
				return url
			}

			shouldFetchPackageImageWith := func(demon bool, pull config.PullPolicy) {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), demon, pull).Return(packageImage, nil)
			}

			when("package image lives in cnb registry", func() {
				var (
					tmpDir          string
					registryFixture string
					packHome        string
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

					packageImage = fakes.NewImage("example.com/some/package@sha256:74eb48882e835d8767f62940d453eb96ed2737de3a16573881dcea7dea769df7", "", nil)
					mockImageFactory.EXPECT().NewImage(packageImage.Name(), false).Return(packageImage, nil)

					bpd := dist.BuildpackDescriptor{
						API:    api.MustParse("0.3"),
						Info:   dist.BuildpackInfo{ID: "example/foo", Version: "1.1.0"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					}

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
							Buildpack: dist.BuildpackURI{URI: createBuildpack(bpd)},
						},
						Publish: true,
					}))
				})

				it.After(func() {
					os.Unsetenv("PACK_HOME")
					err := os.RemoveAll(tmpDir)
					h.AssertNil(t, err)
				})

				when("publish=false and pull-policy=always", func() {
					var configPath string

					it("should pull and use local package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

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

						opts.Publish = false
						opts.PullPolicy = config.PullAlways
						opts.Registry = "some-registry"
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									BuildpackURI: dist.BuildpackURI{
										URI: "urn:cnb:registry:example/foo@1.1.0",
									},
								},
							},
						)

						shouldFetchPackageImageWith(true, config.PullAlways)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})

					it.After(func() {
						os.Unsetenv("PACK_HOME")
						err := os.RemoveAll(tmpDir)
						h.AssertNil(t, err)
					})
				})
			})

			when("package image lives in docker registry", func() {
				it.Before(func() {
					packageImage = fakes.NewImage("docker.io/some/package-"+h.RandString(12), "", nil)
					mockImageFactory.EXPECT().NewImage(packageImage.Name(), false).Return(packageImage, nil)

					bpd := dist.BuildpackDescriptor{
						API:    api.MustParse("0.3"),
						Info:   dist.BuildpackInfo{ID: "some.pkg.bp", Version: "2.3.4", Homepage: "http://meta.buildpack"},
						Stacks: []dist.Stack{{ID: "some.stack.id"}},
					}

					h.AssertNil(t, subject.PackageBuildpack(context.TODO(), pack.PackageBuildpackOptions{
						Name: packageImage.Name(),
						Config: pubbldpkg.Config{
							Buildpack: dist.BuildpackURI{URI: createBuildpack(bpd)},
						},
						Publish:    true,
						PullPolicy: config.PullAlways,
					}))
				})

				prepareFetcherWithMissingPackageImage := func() {
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), packageImage.Name(), gomock.Any(), gomock.Any()).Return(nil, image.ErrNotFound)
				}

				when("publish=false and pull-policy=always", func() {
					it("should pull and use local package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = false
						opts.PullPolicy = config.PullAlways
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(true, config.PullAlways)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=true and pull-policy=always", func() {
					it("should use remote package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = true
						opts.PullPolicy = config.PullAlways
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(false, config.PullAlways)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=true and pull-policy=always", func() {
					it("should use remote package URI", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = true
						opts.PullPolicy = config.PullAlways
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									BuildpackURI: dist.BuildpackURI{URI: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(false, config.PullAlways)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=true and pull-policy=never", func() {
					it("should push to registry and not pull package image", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = true
						opts.PullPolicy = config.PullNever
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						shouldFetchPackageImageWith(false, config.PullNever)
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
				})

				when("publish=false pull-policy=never and there is no local package image", func() {
					it("should fail without trying to retrieve package image from registry", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"

						opts.Publish = false
						opts.PullPolicy = config.PullNever
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.BuildpackConfig{
								ImageOrURI: dist.ImageOrURI{
									ImageRef: dist.ImageRef{ImageName: packageImage.Name()},
								},
							},
						)

						prepareFetcherWithMissingPackageImage()

						h.AssertError(t, subject.CreateBuilder(context.TODO(), opts), "not found")
					})
				})
			})

			when("package image is not a valid package", func() {
				it("should error", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.BuilderName = "some/builder"

					notPackageImage := fakes.NewImage("docker.io/not/package", "", nil)
					opts.Config.Buildpacks = append(
						opts.Config.Buildpacks,
						pubbldr.BuildpackConfig{
							ImageOrURI: dist.ImageOrURI{
								ImageRef: dist.ImageRef{ImageName: notPackageImage.Name()},
							},
						},
					)

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), notPackageImage.Name(), gomock.Any(), gomock.Any()).Return(notPackageImage, nil)
					h.AssertNil(t, notPackageImage.SetLabel("io.buildpacks.buildpack.layers", ""))

					h.AssertError(t, subject.CreateBuilder(context.TODO(), opts), "extracting buildpacks from 'docker.io/not/package': could not find label 'io.buildpacks.buildpackage.metadata'")
				})
			})
		})
	})
}

type fakeBadImageStruct struct {
	*fakes.Image
}

func (i fakeBadImageStruct) Name() string {
	return "fake image"
}

func (i fakeBadImageStruct) Label(str string) (string, error) {
	return "", errors.New("error here")
}
