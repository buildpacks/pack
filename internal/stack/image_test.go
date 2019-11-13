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

func TestStackImage(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()

	spec.Run(t, "testStackImage", testStackImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testStackImage(t *testing.T, when spec.G, it spec.S) {
	var (
		image *fakes.Image
	)

	it.Before(func() {
		image = fakes.NewImage("some/image", "", nil)
	})

	when("#NewImage", func() {
		it("returns an instance when stack ID is present on image", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			stackImage, err := stack.NewImage(image)

			h.AssertNil(t, err)
			h.AssertNotNil(t, stackImage)
		})

		it("returns an error when stack ID is not present on image", func() {
			_, err := stack.NewImage(image)

			h.AssertError(t, err, "image 'some/image' missing label 'io.buildpacks.stack.id'")
		})

		it("returns an error when mixins are not parsable from image", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.mixins", "not json"))

			_, err := stack.NewImage(image)

			h.AssertError(t, err, "unmarshalling label 'io.buildpacks.stack.mixins' from image 'some/image'")
		})
	})

	when("#StackID", func() {
		it("returns stack ID from image", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			stackImage, err := stack.NewImage(image)
			h.AssertNil(t, err)

			stackID := stackImage.StackID()

			h.AssertEq(t, stackID, "some.stack.id")
		})
	})
	
	when("#Mixins", func() {
		it("returns all mixins", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "run:mixinC"]`))
			stackImage, err := stack.NewImage(image)
			h.AssertNil(t, err)

			mixins := stackImage.Mixins()

			h.AssertSliceContainsOnly(t, mixins, "mixinA", "build:mixinB", "run:mixinC")
		})
	})

	when("#CommonMixins", func() {
		it("returns only non-prefixed mixins", func() {
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.id", "some.stack.id"))
			h.AssertNil(t, image.SetLabel("io.buildpacks.stack.mixins", `["mixinA", "build:mixinB", "run:mixinC"]`))
			stackImage, err := stack.NewImage(image)
			h.AssertNil(t, err)

			mixins := stackImage.CommonMixins()

			h.AssertSliceContainsOnly(t, mixins, "mixinA")
		})
	})
}
