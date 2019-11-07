package stack_test

import (
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/internal/stack"
	h "github.com/buildpack/pack/testhelpers"
)

func TestImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testImage", testImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testImage(t *testing.T, when spec.G, it spec.S) {
	when("#CommonMixins", func() {
		it("should return common", func() {
			image := fakes.NewImage("some/image", "", nil)
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.id", `some.stack.id`))
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "mixinB", "run:mixinC"]`))
			stackImage, err := stack.NewImage(image)
			h.AssertNil(t, err)
			
			h.AssertEq(t, stackImage.CommonMixins(), []string{"mixinA", "mixinB"})
		})
	})
}
