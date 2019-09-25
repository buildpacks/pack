package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/blob"
	"github.com/buildpack/pack/builder"
	ifakes "github.com/buildpack/pack/internal/fakes"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/buildpack/pack/testmocks"
)

func TestCreateBuilder(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			mockController     *gomock.Controller
			mockDownloader     *testmocks.MockDownloader
			imageFetcher       *ifakes.FakeImageFetcher
			fakeBuildImage     *fakes.Image
			fakeRunImage       *fakes.Image
			fakeRunImageMirror *fakes.Image
			opts               CreateBuilderOptions
			subject            *Client
			logger             logging.Logger
			out                bytes.Buffer
			tmpDir             string
		)

		it.Before(func() {
			logger = ifakes.NewFakeLogger(&out)
			mockController = gomock.NewController(t)
			mockDownloader = testmocks.NewMockDownloader(mockController)

			fakeBuildImage = fakes.NewImage("some/build-image", "", "")
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_GROUP_ID", "4321"))

			fakeRunImage = fakes.NewImage("some/run-image", "", "")
			h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			fakeRunImageMirror = fakes.NewImage("localhost:5000/some-run-image", "", "")
			h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			imageFetcher = ifakes.NewFakeImageFetcher()
			imageFetcher.LocalImages["some/build-image"] = fakeBuildImage
			imageFetcher.LocalImages["some/run-image"] = fakeRunImage
			imageFetcher.RemoteImages["localhost:5000/some-run-image"] = fakeRunImageMirror

			mockDownloader.EXPECT().Download(gomock.Any(), "https://example.fake/bp-one.tgz").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "some/buildpack/dir").Return(blob.NewBlob(filepath.Join("testdata", "buildpack")), nil).AnyTimes()
			mockDownloader.EXPECT().Download(gomock.Any(), "file:///some-lifecycle").Return(blob.NewBlob(filepath.Join("testdata", "lifecycle")), nil).AnyTimes()

			subject = &Client{
				logger:       logger,
				imageFetcher: imageFetcher,
				downloader:   mockDownloader,
			}

			opts = CreateBuilderOptions{
				BuilderName: "some/builder",
				BuilderConfig: builder.Config{
					Description: "Some description",
					Buildpacks: []builder.BuildpackConfig{
						{
							BuildpackInfo: builder.BuildpackInfo{ID: "bp.one", Version: "1.2.3"},
							URI:           "https://example.fake/bp-one.tgz",
						},
					},
					Order: []builder.OrderEntry{{
						Group: []builder.BuildpackRef{
							{BuildpackInfo: builder.BuildpackInfo{ID: "bp.one", Version: "1.2.3"}, Optional: false},
						}},
					},
					Stack: builder.StackConfig{
						ID:              "some.stack.id",
						BuildImage:      "some/build-image",
						RunImage:        "some/run-image",
						RunImageMirrors: []string{"localhost:5000/some-run-image"},
					},
					Lifecycle: builder.LifecycleConfig{URI: "file:///some-lifecycle"},
				},
				Publish: false,
				NoPull:  false,
			}

			var err error
			tmpDir, err = ioutil.TempDir("", "create-builder-test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			mockController.Finish()
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("validating the builder config", func() {
			it("should fail when the stack ID is empty", func() {
				opts.BuilderConfig.Stack.ID = ""
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "stack.id is required")
			})

			it("should fail when the stack ID from the builder config does not match the stack ID from the build image", func() {
				h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from build image")
			})

			it("should fail when the build image is empty", func() {
				opts.BuilderConfig.Stack.BuildImage = ""
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "stack.build-image is required")
			})

			it("should fail when the run image is empty", func() {
				opts.BuilderConfig.Stack.RunImage = ""
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "stack.run-image is required")
			})

			it("should fail when lifecycle version is not a semver", func() {
				opts.BuilderConfig.Lifecycle.URI = ""
				opts.BuilderConfig.Lifecycle.Version = "not-semver"
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "'lifecycle.version' must be a valid semver")
			})

			it("should fail when both lifecycle version and uri are present", func() {
				opts.BuilderConfig.Lifecycle.URI = "file://some-lifecycle"
				opts.BuilderConfig.Lifecycle.Version = "not-semver"
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "'lifecycle' can only declare 'version' or 'uri', not both")
			})

			it("should fail when buildpack ID does not match downloaded buildpack", func() {
				opts.BuilderConfig.Buildpacks[0].ID = "does.not.match"
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "buildpack from URI 'https://example.fake/bp-one.tgz' has ID 'bp.one' which does not match ID 'does.not.match' from builder config")
			})

			it("should fail when buildpack version does not match downloaded buildpack", func() {
				opts.BuilderConfig.Buildpacks[0].Version = "0.0.0"
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "buildpack from URI 'https://example.fake/bp-one.tgz' has version '1.2.3' which does not match version '0.0.0' from builder config")
			})
		})

		when("validating the run image config", func() {
			it("should fail when the stack ID from the builder config does not match the stack ID from the run image", func() {
				h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from run image 'some/run-image'")
			})

			it("should fail when the stack ID from the builder config does not match the stack ID from the run image mirrors", func() {
				h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "other.stack.id"))

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "stack 'some.stack.id' from builder config is incompatible with stack 'other.stack.id' from run image 'localhost:5000/some-run-image'")
			})

			it("should warn when the run image cannot be found", func() {
				delete(imageFetcher.LocalImages, "some/run-image")

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)

				h.AssertContains(t, out.String(), "Warning: run image 'some/run-image' is not accessible")
			})

			when("publish is true", func() {
				it("should only try to validate the remote run image", func() {
					delete(imageFetcher.LocalImages, "some/run-image")
					delete(imageFetcher.LocalImages, "some/build-image")
					imageFetcher.RemoteImages["some/run-image"] = fakeRunImage
					imageFetcher.RemoteImages["some/build-image"] = fakeBuildImage

					opts.Publish = true
					err := subject.CreateBuilder(context.TODO(), opts)
					h.AssertNil(t, err)
				})
			})
		})

		when("only lifecycle version is provided", func() {
			it.Before(func() {
				opts.BuilderConfig.Lifecycle.URI = ""
				opts.BuilderConfig.Lifecycle.Version = "3.4.5"
			})

			it("should download from predetermined uri", func() {
				mockDownloader.EXPECT().Download(
					gomock.Any(),
					"https://github.com/buildpack/lifecycle/releases/download/v3.4.5/lifecycle-v3.4.5+linux.x86-64.tgz",
				).Return(
					blob.NewBlob(filepath.Join("testdata", "lifecycle")), nil,
				).MinTimes(1)

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})

		when("no lifecycle version or URI is provided", func() {
			it.Before(func() {
				opts.BuilderConfig.Lifecycle.URI = ""
				opts.BuilderConfig.Lifecycle.Version = ""
			})

			it("should download default lifecycle", func() {
				expectedDefaultLifecycleVersion := "0.4.0"
				mockDownloader.EXPECT().Download(
					gomock.Any(),
					fmt.Sprintf(
						"https://github.com/buildpack/lifecycle/releases/download/v%s/lifecycle-v%s+linux.x86-64.tgz",
						expectedDefaultLifecycleVersion,
						expectedDefaultLifecycleVersion,
					),
				).Return(
					blob.NewBlob(filepath.Join("testdata", "lifecycle")), nil,
				).MinTimes(1)

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})

		it("should create a new builder image", func() {
			err := subject.CreateBuilder(context.TODO(), opts)
			h.AssertNil(t, err)

			builderImage, err := builder.GetBuilder(fakeBuildImage)
			h.AssertNil(t, err)

			h.AssertEq(t, builderImage.Name(), "some/builder")
			h.AssertEq(t, builderImage.Description(), "Some description")
			h.AssertEq(t, builderImage.UID, 1234)
			h.AssertEq(t, builderImage.GID, 4321)
			h.AssertEq(t, builderImage.StackID, "some.stack.id")
			bpInfo := builder.BuildpackInfo{
				ID:      "bp.one",
				Version: "1.2.3",
			}
			h.AssertEq(t, builderImage.GetBuildpacks(), []builder.BuildpackMetadata{{
				BuildpackInfo: bpInfo,
				Latest:        true,
			}})
			h.AssertEq(t, builderImage.GetOrder(), builder.Order{{
				Group: []builder.BuildpackRef{{
					BuildpackInfo: bpInfo,
					Optional:      false,
				}},
			}})
			h.AssertEq(t, builderImage.GetLifecycleDescriptor().Info.Version.String(), "3.4.5")

			layerTar, err := fakeBuildImage.FindLayerWithPath("/cnb/lifecycle")
			h.AssertNil(t, err)
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/detector")
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/restorer")
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/analyzer")
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/builder")
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/exporter")
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/cacher")
			assertTarHasFile(t, layerTar, "/cnb/lifecycle/launcher")
		})

		when("windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
			})

			it("disallows directory-based buildpacks", func() {
				opts.BuilderConfig.Buildpacks[0].URI = "testdata/buildpack"

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
				opts.BuilderConfig.Buildpacks[0].URI = "some/buildpack/dir"

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertNil(t, err)
			})
		})
	})
}

func assertTarHasFile(t *testing.T, tarFile, path string) {
	t.Helper()

	exist := tarHasFile(t, tarFile, path)
	if !exist {
		t.Fatalf("%s does not exist in %s", path, tarFile)
	}
}

func tarHasFile(t *testing.T, tarFile, path string) (exist bool) {
	t.Helper()

	r, err := os.Open(tarFile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			return true
		}
	}

	return false
}
