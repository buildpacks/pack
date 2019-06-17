package pack_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/buildpack/imgutil/fakes"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/logging"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	imocks "github.com/buildpack/pack/internal/mocks"
	"github.com/buildpack/pack/lifecycle"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCreateBuilder(t *testing.T) {
	h.RequireDocker(t)
	color.NoColor = true
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			mockController       *gomock.Controller
			mockBPFetcher        *mocks.MockBuildpackFetcher
			mockLifecycleFetcher *mocks.MockLifecycleFetcher
			imageFetcher         *imocks.FakeImageFetcher
			fakeBuildImage       *fakes.Image
			fakeRunImage         *fakes.Image
			fakeRunImageMirror   *fakes.Image
			opts                 pack.CreateBuilderOptions
			subject              *pack.Client
			log                  logging.Logger
			out                  bytes.Buffer
		)

		it.Before(func() {
			log = imocks.NewMockLogger(&out)
			mockController = gomock.NewController(t)
			mockBPFetcher = mocks.NewMockBuildpackFetcher(mockController)
			mockLifecycleFetcher = mocks.NewMockLifecycleFetcher(mockController)

			fakeBuildImage = fakes.NewImage("some/build-image", "", "")
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_GROUP_ID", "4321"))

			fakeRunImage = fakes.NewImage("some/run-image", "", "")
			h.AssertNil(t, fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			fakeRunImageMirror = fakes.NewImage("localhost:5000/some-run-image", "", "")
			h.AssertNil(t, fakeRunImageMirror.SetLabel("io.buildpacks.stack.id", "some.stack.id"))

			imageFetcher = imocks.NewFakeImageFetcher()
			imageFetcher.LocalImages["some/build-image"] = fakeBuildImage
			imageFetcher.LocalImages["some/run-image"] = fakeRunImage
			imageFetcher.RemoteImages["localhost:5000/some-run-image"] = fakeRunImageMirror

			bp := buildpack.Buildpack{
				ID:      "bp.one",
				Latest:  true,
				Path:    filepath.Join("testdata", "buildpack"),
				Version: "1.2.3",
				Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
			}

			mockBPFetcher.EXPECT().FetchBuildpack(gomock.Any()).Return(bp, nil).AnyTimes()

			mockLifecycleFetcher.EXPECT().Fetch(gomock.Any(), gomock.Any()).
				Return(lifecycle.Metadata{
					Path:    filepath.Join("testdata", "lifecycle.tgz"),
					Version: semver.MustParse("3.4.5"),
				}, nil).AnyTimes()

			subject = pack.NewClient(
				&config.Config{},
				log,
				imageFetcher,
				mockBPFetcher,
				mockLifecycleFetcher,
				nil,
				nil,
			)

			opts = pack.CreateBuilderOptions{
				BuilderName: "some/builder",
				BuilderConfig: builder.Config{
					Description: "Some description",
					Buildpacks: []builder.BuildpackConfig{
						{
							ID:      "bp.one",
							Version: "1.2.3",
							URI:     "https://example.fake/bp-one.tgz",
							Latest:  true,
						},
					},
					Groups: []builder.GroupMetadata{{
						Buildpacks: []builder.GroupBuildpack{
							{ID: "bp.one", Version: "1.2.3", Optional: false},
						}},
					},
					Stack: builder.StackConfig{
						ID:              "some.stack.id",
						BuildImage:      "some/build-image",
						RunImage:        "some/run-image",
						RunImageMirrors: []string{"localhost:5000/some-run-image"},
					},
					Lifecycle: builder.LifecycleConfig{Version: "3.4.5"},
				},
				Publish: false,
				NoPull:  false,
			}
		})

		it.After(func() {
			mockController.Finish()
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
				opts.BuilderConfig.Lifecycle.Version = "not-semver"
				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "lifecycle.version must be a valid semver")
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
			h.AssertEq(t, builderImage.GetBuildpacks(), []builder.BuildpackMetadata{{
				ID:      "bp.one",
				Version: "1.2.3",
				Latest:  true,
			}})
			h.AssertEq(t, builderImage.GetOrder(), []builder.GroupMetadata{{
				Buildpacks: []builder.GroupBuildpack{{
					ID:       "bp.one",
					Version:  "1.2.3",
					Optional: false,
				}},
			}})
			h.AssertEq(t, builderImage.GetLifecycleVersion().String(), "3.4.5")

			layerTar, err := fakeBuildImage.FindLayerWithPath("/lifecycle")
			h.AssertNil(t, err)
			assertTarHasFile(t, layerTar, "/lifecycle/detector")
			assertTarHasFile(t, layerTar, "/lifecycle/restorer")
			assertTarHasFile(t, layerTar, "/lifecycle/analyzer")
			assertTarHasFile(t, layerTar, "/lifecycle/builder")
			assertTarHasFile(t, layerTar, "/lifecycle/exporter")
			assertTarHasFile(t, layerTar, "/lifecycle/cacher")
			assertTarHasFile(t, layerTar, "/lifecycle/launcher")
		})

		when("windows", func() {
			it.Before(func() {
				h.SkipIf(t, runtime.GOOS != "windows", "Skipped on non-windows")
			})

			it("only allows tgz buildpacks", func() {
				opts.BuilderConfig.Buildpacks[0].URI = "some/buildpack/dir"

				err := subject.CreateBuilder(context.TODO(), opts)
				h.AssertError(t, err, "buildpack 'bp.one': Windows only supports .tgz-based buildpacks")
			})
		})

		when("is *nix", func() {
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
