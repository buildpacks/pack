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

func TestRunImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testRunImage", testRunImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRunImage(t *testing.T, when spec.G, it spec.S) {
	var (
		ctrl *gomock.Controller
	)
	
	it.Before(func() {
		ctrl = gomock.NewController(t)
	})

	it.After(func() {
		ctrl.Finish()
	})

	when("#NewRunImage", func() {
		it("returns an instance when mixins are only common or 'run:'-prefixed", func() {
			image := testmocks.NewMockImage(ctrl)
			image.EXPECT().Mixins().Return([]string{"mixinA", "run:mixinB"})

			runImage, err := stack.NewRunImage(image)

			h.AssertNil(t, err)
			h.AssertNotNil(t, runImage)
		})

		it("returns an error when any mixins are 'build:'-prefixed", func() {
			image := testmocks.NewMockImage(ctrl)
			image.EXPECT().Mixins().Return([]string{"mixinA", "build:mixinB", "build:mixinC"})
			image.EXPECT().Name().Return("some/image")
			
			_, err := stack.NewRunImage(image)

			h.AssertError(t, err, "'some/image' contains build-only mixin(s): build:mixinB, build:mixinC")
		})
	})

	when("#RunOnlyMixins", func() {
		// TODO
	})
}
