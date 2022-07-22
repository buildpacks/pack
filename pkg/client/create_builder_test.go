package client_test

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

	pubbldr "github.com/buildpacks/pack/builder"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/builder"
	ifakes "github.com/buildpacks/pack/internal/fakes"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildmodule"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/buildpacks/pack/pkg/testmocks"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestCreateBuilder(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			mockController          *gomock.Controller
			mockDownloader          *testmocks.MockBlobDownloader
			mockBuildpackDownloader *testmocks.MockBuildpackDownloader
			mockImageFactory        *testmocks.MockImageFactory
			mockImageFetcher        *testmocks.MockImageFetcher
			mockDockerClient        *testmocks.MockCommonAPIClient
			fakeBuildImage          *fakes.Image
			fakeRunImage            *fakes.Image
			fakeRunImageMirror      *fakes.Image
			opts                    client.CreateBuilderOptions
			subject                 *client.Client
			logger                  logging.Logger
			out                     bytes.Buffer
			tmpDir                  string
		)
		var prepareFetcherWithRunImages = func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", gomock.Any()).Return(fakeRunImage, nil).AnyTimes()
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", gomock.Any()).Return(fakeRunImageMirror, nil).AnyTimes()
		}

		var prepareFetcherWithBuildImage = func() {
			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", gomock.Any()).Return(fakeBuildImage, nil)
		}

		var createBuildpack = func(descriptor dist.BuildpackDescriptor) buildmodule.BuildModule {
			buildpack, err := ifakes.NewFakeBuildpack(descriptor, 0644)
			h.AssertNil(t, err)
			return buildpack
		}

		var shouldCallBuildpackDownloaderWith = func(uri string, buildpackDownloadOptions buildmodule.DownloadOptions) {
			buildpack := createBuildpack(dist.BuildpackDescriptor{
				WithAPI:    api.MustParse("0.3"),
				WithInfo:   dist.ModuleInfo{ID: "example/foo", Version: "1.1.0"},
				WithStacks: []dist.Stack{{ID: "some.stack.id"}},
			})
			mockBuildpackDownloader.EXPECT().Download(gomock.Any(), uri, gomock.Any()).Return(buildpack, nil, nil)
		}

		it.Before(func() {
			logger = logging.NewLogWithWriters(&out, &out, logging.WithVerbose())
			mockController = gomock.NewController(t)
			mockDownloader = testmocks.NewMockBlobDownloader(mockController)
			mockImageFetcher = testmocks.NewMockImageFetcher(mockController)
			mockImageFactory = testmocks.NewMockImageFactory(mockController)
			mockDockerClient = testmocks.NewMockCommonAPIClient(mockController)
			mockBuildpackDownloader = testmocks.NewMockBuildpackDownloader(mockController)

			fakeBuildImage = fakes.NewImage("some/build-image", "", nil)
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.mixins", `["mixinX", "build:mixinY"]`))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_GROUP_ID", "4321"))

			fakeRunImage = fakes.NewImage("some/run-image", "", nil)
			h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			fakeRunImageMirror = fakes.NewImage("localhost:5000/some/run-image", "", nil)
			h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			exampleBuildpackBlob := blob.NewBlob(filepath.Join("testdata", "buildpack"))
			mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz").Return(exampleBuildpackBlob, nil).AnyTimes()
			exampleExtensionBlob := blob.NewBlob(filepath.Join("testdata", "extension"))
			mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/ext-one.tgz").Return(exampleExtensionBlob, nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "some/buildpack/dir").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle", "platform-0.4")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle-platform-0-1").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle-platform-0.1")), nil).AnyTimes()

			bp, err := buildmodule.FromBuildpackRootBlob(exampleBuildpackBlob, archive.DefaultTarWriterFactory())
			h.AssertNil(t, err)
			mockBuildpackDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz", gomock.Any()).Return(bp, nil, nil).AnyTimes()
			ext, err := buildmodule.FromExtensionRootBlob(exampleExtensionBlob, archive.DefaultTarWriterFactory())
			h.AssertNil(t, err)
			mockBuildpackDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/ext-one.tgz", gomock.Any()).Return(ext, nil, nil).AnyTimes()

			subject, err = client.NewClient(
				client.WithLogger(logger),
				client.WithDownloader(mockDownloader),
				client.WithImageFactory(mockImageFactory),
				client.WithFetcher(mockImageFetcher),
				client.WithDockerClient(mockDockerClient),
				client.WithBuildpackDownloader(mockBuildpackDownloader),
			)
			h.AssertNil(t, err)

			mockDockerClient.EXPECT().Info(context.TODO()).Return(types.Info{OSType: "linux"}, nil).AnyTimes()

			opts = client.CreateBuilderOptions{
				RelativeBaseDir: "/",
				BuilderName:     "some/builder",
				Config: pubbldr.Config{
					Description: "Some description",
					Buildpacks: []pubbldr.ModuleConfig{
						{
							ModuleInfo: dist.ModuleInfo{ID: "bp.one", Version: "1.2.3", Homepage: "http://one.buildpack"},
							ImageOrURI: dist.ImageOrURI{
								BuildpackURI: dist.BuildpackURI{
									URI: "https://example.fake/bp-one.tgz",
								},
							},
						},
					},
					Extensions: []pubbldr.ModuleConfig{
						{
							ModuleInfo: dist.ModuleInfo{ID: "ext.one", Version: "1.2.3", Homepage: "http://one.extension"},
							ImageOrURI: dist.ImageOrURI{
								BuildpackURI: dist.BuildpackURI{
									URI: "https://example.fake/ext-one.tgz",
								},
							},
						},
					},
					Order: []dist.OrderEntry{{
						Group: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "bp.one", Version: "1.2.3"}, Optional: false},
						}},
					},
					OrderExtensions: []dist.OrderEntry{{
						Group: []dist.ModuleRef{
							{ModuleInfo: dist.ModuleInfo{ID: "ext.one", Version: "1.2.3"}, Optional: true}, // TODO: extensions are always optional
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
				PullPolicy: image.PullAlways,
			}

			tmpDir, err = ioutil.TempDir("", "create-builder-test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

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
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(fakeBuildImage, nil)
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

			it("should fail when extension ID does not match downloaded extension", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Extensions[0].ID = "does.not.match"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "extension from URI 'https://example.fake/ext-one.tgz' has ID 'ext.one' which does not match ID 'does.not.match' from builder config")
			})

			it("should fail when extension version does not match downloaded extension", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Extensions[0].Version = "0.0.0"

				err := subject.CreateBuilder(context.TODO(), opts)

				h.AssertError(t, err, "extension from URI 'https://example.fake/ext-one.tgz' has version '1.2.3' which does not match version '0.0.0' from builder config")
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
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(fakeBuildImage, nil)

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", image.FetchOptions{Daemon: false, PullPolicy: image.PullAlways}).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", image.FetchOptions{Daemon: false, PullPolicy: image.PullAlways}).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(nil, errors.Wrap(image.ErrNotFound, "yikes"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)

				h.AssertContains(t, out.String(), "Warning: run image 'some/run-image' is not accessible")
			})

			it("should fail when not publish and the run image cannot be fetched", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(nil, errors.New("yikes"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "failed to fetch image: yikes")
			})

			it("should fail when publish and the run image cannot be fetched", func() {
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", image.FetchOptions{Daemon: false, PullPolicy: image.PullAlways}).Return(nil, errors.New("yikes"))

				opts.Publish = true
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "failed to fetch image: yikes")
			})

			it("should fail when the run image isn't a valid image", func() {
				fakeImage := fakeBadImageStruct{}

				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", gomock.Any()).Return(fakeImage, nil).AnyTimes()
				mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", gomock.Any()).Return(fakeImage, nil).AnyTimes()

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "failed to label image")
			})

			when("publish is true", func() {
				it("should only try to validate the remote run image", func() {
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true}).Times(0)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", image.FetchOptions{Daemon: true}).Times(0)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", image.FetchOptions{Daemon: true}).Times(0)

					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: false}).Return(fakeBuildImage, nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/run-image", image.FetchOptions{Daemon: false}).Return(fakeRunImage, nil)
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "localhost:5000/some/run-image", image.FetchOptions{Daemon: false}).Return(fakeRunImageMirror, nil)

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
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(nil, image.ErrNotFound)

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "fetch build image: not found")
				})
			})

			when("build image isn't a valid image", func() {
				it("should fail", func() {
					fakeImage := fakeBadImageStruct{}

					prepareFetcherWithRunImages()
					mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(fakeImage, nil)

					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "failed to create builder: invalid build-image")
				})
			})

			when("windows containers", func() {
				when("experimental enabled", func() {
					it("succeeds", func() {
						opts.Config.Extensions = nil      // TODO: downloading extensions doesn't work yet
						opts.Config.OrderExtensions = nil // TODO: downloading extensions doesn't work yet
						packClientWithExperimental, err := client.NewClient(
							client.WithLogger(logger),
							client.WithDownloader(mockDownloader),
							client.WithImageFactory(mockImageFactory),
							client.WithFetcher(mockImageFetcher),
							client.WithExperimental(true),
						)
						h.AssertNil(t, err)

						prepareFetcherWithRunImages()

						h.AssertNil(t, fakeBuildImage.SetOS("windows"))
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(fakeBuildImage, nil)

						err = packClientWithExperimental.CreateBuilder(context.TODO(), opts)
						h.AssertNil(t, err)
					})
				})

				when("experimental disabled", func() {
					it("fails", func() {
						prepareFetcherWithRunImages()

						h.AssertNil(t, fakeBuildImage.SetOS("windows"))
						mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", image.FetchOptions{Daemon: true, PullPolicy: image.PullAlways}).Return(fakeBuildImage, nil)

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

					uri, err := paths.FilePathToURI(opts.Config.Lifecycle.URI, opts.RelativeBaseDir)
					h.AssertNil(t, err)

					mockDownloader.EXPECT().Download(gomock.Any(), uri).Return(nil, errors.New("error here")).AnyTimes()

					err = subject.CreateBuilder(context.TODO(), opts)
					h.AssertError(t, err, "downloading lifecycle")
				})
			})

			when("lifecycle isn't a valid lifecycle", func() {
				it("should fail", func() {
					prepareFetcherWithBuildImage()
					prepareFetcherWithRunImages()
					opts.Config.Lifecycle.URI = "fake"

					uri, err := paths.FilePathToURI(opts.Config.Lifecycle.URI, opts.RelativeBaseDir)
					h.AssertNil(t, err)

					mockDownloader.EXPECT().Download(gomock.Any(), uri).Return(blob.NewBlob(filepath.Join("testdata", "empty-file")), nil).AnyTimes()

					err = subject.CreateBuilder(context.TODO(), opts)
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
					opts.Config.Extensions = nil      // TODO: downloading extensions doesn't work yet
					opts.Config.OrderExtensions = nil // TODO: downloading extensions doesn't work yet
					packClientWithExperimental, err := client.NewClient(
						client.WithLogger(logger),
						client.WithDownloader(mockDownloader),
						client.WithImageFactory(mockImageFactory),
						client.WithFetcher(mockImageFetcher),
						client.WithExperimental(true),
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
					opts.Config.Extensions = nil      // TODO: downloading extensions doesn't work yet
					opts.Config.OrderExtensions = nil // TODO: downloading extensions doesn't work yet
					packClientWithExperimental, err := client.NewClient(
						client.WithLogger(logger),
						client.WithDownloader(mockDownloader),
						client.WithImageFactory(mockImageFactory),
						client.WithFetcher(mockImageFetcher),
						client.WithExperimental(true),
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

				bpInfo := dist.ModuleInfo{
					ID:       "bp.one",
					Version:  "1.2.3",
					Homepage: "http://one.buildpack",
				}
				h.AssertEq(t, bldr.Buildpacks(), []dist.ModuleInfo{bpInfo})
				bpInfo.Homepage = ""
				h.AssertEq(t, bldr.Order(), dist.Order{{
					Group: []dist.ModuleRef{{
						ModuleInfo: bpInfo,
						Optional:   false,
					}},
				}})
			})

			it("should set extensions and order-extensions metadata", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()

				bldr := successfullyCreateBuilder()

				extInfo := dist.ModuleInfo{
					ID:       "ext.one",
					Version:  "1.2.3",
					Homepage: "http://one.extension",
				}
				h.AssertEq(t, bldr.Extensions(), []dist.ModuleInfo{extInfo})
				extInfo.Homepage = ""
				h.AssertEq(t, bldr.OrderExtensions(), dist.Order{{
					Group: []dist.ModuleRef{{
						ModuleInfo: extInfo,
						Optional:   true,
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
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Supported.AsStrings(), []string{"0.2", "0.3", "0.4", "0.9"})
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Platform.Deprecated.AsStrings(), []string{"0.2"})
				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Platform.Supported.AsStrings(), []string{"0.3", "0.4"})
			})

			it("should warn when deprecated Buildpack API version is used", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Deprecated.AsStrings(), []string{"0.2", "0.3"})
				h.AssertContains(t, out.String(), fmt.Sprintf("Buildpack %s is using deprecated Buildpacks API version %s", style.Symbol("bp.one@1.2.3"), style.Symbol("0.3")))
				h.AssertContains(t, out.String(), fmt.Sprintf("Extension %s is using deprecated Buildpacks API version %s", style.Symbol("ext.one@1.2.3"), style.Symbol("0.3")))
			})

			it("shouldn't warn when Buildpack API version used isn't deprecated", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				opts.Config.Buildpacks[0].URI = "https://example.fake/bp-one-with-api-4.tgz"
				opts.Config.Extensions[0].URI = "https://example.fake/ext-one-with-api-9.tgz"

				buildpackBlob := blob.NewBlob(filepath.Join("testdata", "buildpack-api-0.4"))
				buildpack, err := buildmodule.FromBuildpackRootBlob(buildpackBlob, archive.DefaultTarWriterFactory())
				h.AssertNil(t, err)
				mockBuildpackDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one-with-api-4.tgz", gomock.Any()).Return(buildpack, nil, nil)

				extensionBlob := blob.NewBlob(filepath.Join("testdata", "extension-api-0.9"))
				extension, err := buildmodule.FromExtensionRootBlob(extensionBlob, archive.DefaultTarWriterFactory())
				h.AssertNil(t, err)
				mockBuildpackDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/ext-one-with-api-9.tgz", gomock.Any()).Return(extension, nil, nil)

				bldr := successfullyCreateBuilder()

				h.AssertEq(t, bldr.LifecycleDescriptor().APIs.Buildpack.Deprecated.AsStrings(), []string{"0.2", "0.3"})
				h.AssertNotContains(t, out.String(), "is using deprecated Buildpacks API version")
			})
		})

		it("supports directory buildpacks", func() {
			prepareFetcherWithBuildImage()
			prepareFetcherWithRunImages()
			opts.RelativeBaseDir = ""
			directoryPath := "testdata/buildpack"
			opts.Config.Buildpacks[0].URI = directoryPath

			buildpackBlob := blob.NewBlob(directoryPath)
			buildpack, err := buildmodule.FromBuildpackRootBlob(buildpackBlob, archive.DefaultTarWriterFactory())
			h.AssertNil(t, err)
			mockBuildpackDownloader.EXPECT().Download(gomock.Any(), directoryPath, gomock.Any()).Return(buildpack, nil, nil)

			err = subject.CreateBuilder(context.TODO(), opts)
			h.AssertNil(t, err)
		})

		it("supports directory extensions", func() {
			prepareFetcherWithBuildImage()
			prepareFetcherWithRunImages()
			opts.RelativeBaseDir = ""
			directoryPath := "testdata/extension"
			opts.Config.Extensions[0].URI = directoryPath

			extensionBlob := blob.NewBlob(directoryPath)
			extension, err := buildmodule.FromExtensionRootBlob(extensionBlob, archive.DefaultTarWriterFactory())
			h.AssertNil(t, err)
			mockBuildpackDownloader.EXPECT().Download(gomock.Any(), directoryPath, gomock.Any()).Return(extension, nil, nil)

			err = subject.CreateBuilder(context.TODO(), opts)
			h.AssertNil(t, err)
		})

		when("package file", func() {
			it.Before(func() {
				fileURI := func(path string) (original, uri string) {
					absPath, err := paths.FilePathToURI(path, "")
					h.AssertNil(t, err)
					return path, absPath
				}

				cnbFile, _ := fileURI(filepath.Join(tmpDir, "bp_one1.cnb"))
				buildpackPath, buildpackPathURI := fileURI(filepath.Join("testdata", "buildpack"))
				mockDownloader.EXPECT().Download(gomock.Any(), buildpackPathURI).Return(blob.NewBlob(buildpackPath), nil)

				h.AssertNil(t, subject.PackageBuildpack(context.TODO(), client.PackageBuildpackOptions{
					Name: cnbFile,
					Config: pubbldpkg.Config{
						Platform:  dist.Platform{OS: "linux"},
						Buildpack: dist.BuildpackURI{URI: buildpackPath},
					},
					Format: "file",
				}))

				buildpack, _, err := buildmodule.BuildpacksFromOCILayoutBlob(blob.NewBlob(cnbFile))
				h.AssertNil(t, err)
				mockBuildpackDownloader.EXPECT().Download(gomock.Any(), cnbFile, gomock.Any()).Return(buildpack, nil, nil).AnyTimes()
				opts.Config.Buildpacks = []pubbldr.ModuleConfig{{
					ImageOrURI: dist.ImageOrURI{BuildpackURI: dist.BuildpackURI{URI: cnbFile}},
				}}
			})

			it("package file is valid", func() {
				prepareFetcherWithBuildImage()
				prepareFetcherWithRunImages()
				bldr := successfullyCreateBuilder()

				bpInfo := dist.ModuleInfo{
					ID:       "bp.one",
					Version:  "1.2.3",
					Homepage: "http://one.buildpack",
				}
				h.AssertEq(t, bldr.Buildpacks(), []dist.ModuleInfo{bpInfo})
				bpInfo.Homepage = ""
				h.AssertEq(t, bldr.Order(), dist.Order{{
					Group: []dist.ModuleRef{{
						ModuleInfo: bpInfo,
						Optional:   false,
					}},
				}})
			})
		})

		when("packages", func() {
			when("package image lives in cnb registry", func() {
				when("publish=false and pull-policy=always", func() {
					it("should call BuildpackDownloader with the proper argumentss", func() {
						prepareFetcherWithBuildImage()
						prepareFetcherWithRunImages()
						opts.BuilderName = "some/builder"
						opts.Publish = false
						opts.PullPolicy = image.PullAlways
						opts.Registry = "some-registry"
						opts.Config.Buildpacks = append(
							opts.Config.Buildpacks,
							pubbldr.ModuleConfig{
								ImageOrURI: dist.ImageOrURI{
									BuildpackURI: dist.BuildpackURI{
										URI: "urn:cnb:registry:example/foo@1.1.0",
									},
								},
							},
						)

						shouldCallBuildpackDownloaderWith("urn:cnb:registry:example/foo@1.1.0", buildmodule.DownloadOptions{Daemon: true, PullPolicy: image.PullAlways, RegistryName: "some-"})
						h.AssertNil(t, subject.CreateBuilder(context.TODO(), opts))
					})
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
