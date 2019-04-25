package pack_test

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestCreateBuilder(t *testing.T) {
	h.RequireDocker(t)
	color.NoColor = true
	if runtime.GOOS == "windows" {
		t.Skip("create builder is not implemented on windows")
	}
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			mockController   *gomock.Controller
			mockImageFetcher *mocks.MockImageFetcher
			mockBPFetcher    *mocks.MockBuildpackFetcher
			fakeBuildImage   *fakes.Image
			logOut, logErr   *bytes.Buffer
			opts             pack.CreateBuilderOptions
			subject          *pack.Client
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImageFetcher = mocks.NewMockImageFetcher(mockController)
			mockBPFetcher = mocks.NewMockBuildpackFetcher(mockController)

			fakeBuildImage = fakes.NewImage(t, "some/build-image", "", "")
			h.AssertNil(t, fakeBuildImage.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_USER_ID", "1234"))
			h.AssertNil(t, fakeBuildImage.SetEnv("CNB_GROUP_ID", "4321"))

			mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", gomock.Any(), gomock.Any()).
				Return(fakeBuildImage, nil).AnyTimes()

			bp := buildpack.Buildpack{
				ID:      "bp.one",
				Latest:  true,
				Dir:     filepath.Join("testdata", "buildpack"),
				Version: "1.2.3",
				Stacks:  []buildpack.Stack{{ID: "some.stack.id"}},
			}

			mockBPFetcher.EXPECT().FetchBuildpack(gomock.Any()).Return(bp, nil).AnyTimes()

			logOut, logErr = &bytes.Buffer{}, &bytes.Buffer{}

			subject = pack.NewClient(
				&config.Config{},
				logging.NewLogger(logOut, logErr, true, false),
				mockImageFetcher,
				nil,
				mockBPFetcher,
				nil,
			)

			opts = pack.CreateBuilderOptions{
				BuilderName: "some/builder",
				BuilderConfig: builder.Config{
					Buildpacks: []builder.BuildpackConfig{
						{ID: "bp.one", URI: "https://example.fake/bp-one.tgz", Latest: true},
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
						RunImageMirrors: nil,
					},
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
		})

		it("should create a new builder image", func() {
			err := subject.CreateBuilder(context.TODO(), opts)
			h.AssertNil(t, err)

			builderImage, err := builder.GetBuilder(fakeBuildImage)
			h.AssertNil(t, err)

			h.AssertEq(t, builderImage.Name(), "some/builder")
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
		})
	})
}
