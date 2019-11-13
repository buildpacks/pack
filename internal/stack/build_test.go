package stack_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/stack"
	"github.com/buildpack/pack/internal/stack/testmocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuildImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testBuildImage", testBuildImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildImage(t *testing.T, when spec.G, it spec.S) {
	var (
		ctrl  *gomock.Controller
		image *testmocks.MockImage
	)

	it.Before(func() {
		ctrl = gomock.NewController(t)
		image = testmocks.NewMockImage(ctrl)
	})

	it.After(func() {
		ctrl.Finish()
	})

	when("#NewBuildImage", func() {
		it("returns an instance when image is valid", func() {
			image.EXPECT().Mixins().Return([]string{"mixinA", "build:mixinB"})

			buildImage, err := stack.NewBuildImage(image)

			h.AssertNil(t, err)
			h.AssertNotNil(t, buildImage)
		})

		it("returns an error when any mixins are 'run:'-prefixed", func() {
			image.EXPECT().Mixins().Return([]string{"mixinA", "run:mixinB", "run:mixinC"})
			image.EXPECT().Name().Return("some/image")

			_, err := stack.NewBuildImage(image)

			h.AssertError(t, err, "'some/image' contains run-only mixin(s): run:mixinB, run:mixinC")
		})
	})

	when("#BuildOnlyMixins", func() {
		it("returns only mixins prefixed with 'build:' from image label", func() {
			image.EXPECT().Mixins().Return([]string{"mixinA", "build:mixinB", "build:mixinC"}).AnyTimes()
			buildImage, err := stack.NewBuildImage(image)
			h.AssertNil(t, err)

			buildOnlyMixins := buildImage.BuildOnlyMixins()

			h.AssertSliceContainsOnly(t, buildOnlyMixins, "build:mixinB", "build:mixinC")
		})
	})
}
