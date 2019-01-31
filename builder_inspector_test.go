package pack_test

import (
	"testing"

	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestInspectBuilder(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "inspect-builder", testInspectBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testInspectBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		inspector        *pack.BuilderInspect
		mockController   *gomock.Controller
		mockBuilderImage *mocks.MockImage
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockBuilderImage = mocks.NewMockImage(mockController)
		mockBuilderImage.EXPECT().Name().Return("some/builder").AnyTimes()

		inspector = &pack.BuilderInspect{
			Config: &config.Config{},
		}
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#Inspect", func() {
		when("builder has valid metadata label", func() {
			it.Before(func() {
				mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"runImage": {"image": "some/default", "mirrors": ["gcr.io/some/default"]}}`, nil)
			})

			when("builder exists in config", func() {
				it.Before(func() {
					inspector.Config.RunImages = []config.RunImage{
						{
							Image:   "some/default",
							Mirrors: []string{"gcr.io/some/run"},
						},
					}
				})

				it("returns the builder with the given name", func() {
					builder, err := inspector.Inspect(mockBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, builder.Image, "some/builder")
				})

				it("set the correct run image", func() {
					builder, err := inspector.Inspect(mockBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, builder.RunImage, "some/default")
				})

				it("set the local run image mirrors", func() {
					builder, err := inspector.Inspect(mockBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, builder.LocalRunImageMirrors, []string{"gcr.io/some/run"})
				})

				it("set the defaults run image mirrors", func() {
					builder, err := inspector.Inspect(mockBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, builder.RunImageMirrors, []string{"gcr.io/some/default"})
				})
			})

			when("builder does not exist in config", func() {
				it("returns the builder with default run images only", func() {
					builder, err := inspector.Inspect(mockBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, builder.Image, "some/builder")
					h.AssertEq(t, len(builder.LocalRunImageMirrors), 0)
					h.AssertEq(t, builder.RunImageMirrors, []string{"gcr.io/some/default"})
				})
			})
		})

		when("builder has missing metadata label", func() {
			it.Before(func() {
				mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			})

			it("returns an error", func() {
				_, err := inspector.Inspect(mockBuilderImage)
				h.AssertError(t, err, "invalid builder image 'some/builder': missing required label 'io.buildpacks.builder.metadata' -- try recreating builder")
			})
		})

		when("builder has invalid metadata label", func() {
			it.Before(func() {
				mockBuilderImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("junk", nil)
			})

			it("returns an error", func() {
				_, err := inspector.Inspect(mockBuilderImage)
				h.AssertNotNil(t, err)
				h.AssertContains(t, err.Error(), "failed to parse run images for builder 'some/builder':")
			})
		})
	})
}
