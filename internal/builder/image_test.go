package builder_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/builder"
	"github.com/buildpack/pack/internal/builder/testmocks"
	"github.com/buildpack/pack/internal/dist"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuilderImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testBuilderImage", testBuilderImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilderImage(t *testing.T, when spec.G, it spec.S) {
	var (
		ctrl  *gomock.Controller
		image *testmocks.MockBuildImage
	)

	it.Before(func() {
		ctrl = gomock.NewController(t)
		image = testmocks.NewMockBuildImage(ctrl)
	})

	it.After(func() {
		ctrl.Finish()
	})

	when("#NewImage", func() {
		it("returns an instance when image is valid", func() {
			image.EXPECT().Label(gomock.Any()).Return("", nil).AnyTimes()
			builderImage, err := builder.NewImage(image)

			h.AssertNil(t, err)
			h.AssertNotNil(t, builderImage)
		})

		it("returns an error when metadata is not parsable from image", func() {
			image.EXPECT().Label("io.buildpacks.builder.metadata").Return("not json", nil)
			image.EXPECT().Name().Return("some/image")

			_, err := builder.NewImage(image)

			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.builder.metadata' from image 'some/image'")
		})

		it("returns an error when order is not parsable from image", func() {
			image.EXPECT().Label("io.buildpacks.buildpack.order").Return("not json", nil)
			image.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			image.EXPECT().Name().Return("some/image")

			_, err := builder.NewImage(image)

			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.buildpack.order' from image 'some/image'")
		})

		it("falls back to metadata groups when order label is not present on image", func() {
			image.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{"groups": [{"buildpacks": [{"id": "some.buildpack.id", "version": "some.buildpack.version"}]}]}`, nil)
			image.EXPECT().Label("io.buildpacks.buildpack.order").Return("", nil)
			image.EXPECT().Label("io.buildpacks.buildpack.layers").Return("", nil)

			builderImage, err := builder.NewImage(image)
			h.AssertNil(t, err)

			h.AssertEq(t, builderImage.Order(), dist.Order{{
				Group: []dist.BuildpackRef{{
					BuildpackInfo: dist.BuildpackInfo{ID: "some.buildpack.id", Version: "some.buildpack.version"},
				}},
			}})
		})

		it("returns an error when layer info is not parsable from image", func() {
			image.EXPECT().Label("io.buildpacks.buildpack.layers").Return("not json", nil)
			image.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			image.EXPECT().Label("io.buildpacks.buildpack.order").Return("", nil)
			image.EXPECT().Name().Return("some/image")

			_, err := builder.NewImage(image)

			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.buildpack.layers' from image 'some/image'")
		})
	})

	when("#Metadata", func() {
		// TODO
	})

	when("#SetMetadata", func() {
		// TODO
	})

	when("#Order", func() {
		// TODO
	})

	when("#SetOrder", func() {
		// TODO
	})

	when("#BuildpackLayers", func() {
		// TODO
	})

	when("#SetBuildpackLayers", func() {
		// TODO
	})
}
