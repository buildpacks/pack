package stack_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/stack"
	h "github.com/buildpack/pack/testhelpers"
)

func TestMixinValidation(t *testing.T) {
	color.Disable(true)
	defer func() { color.Disable(false) }()
	spec.Run(t, "testMixinValidation", testMixinValidation, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testMixinValidation(t *testing.T, when spec.G, it spec.S) {
	when("#ValidateMixins", func() {
		it("ignores stage-specific mixins", func() {
			buildMixins := []string{"mixinA", "build:mixinB"}
			runMixins := []string{"mixinA", "run:mixinC"}

			h.AssertNil(t, stack.ValidateMixins("some/build", buildMixins, "some/run", runMixins))
		})

		it("allows extraneous run image mixins", func() {
			buildMixins := []string{"mixinA"}
			runMixins := []string{"mixinA", "mixinB"}

			h.AssertNil(t, stack.ValidateMixins("some/build", buildMixins, "some/run", runMixins))
		})

		it("returns an error with any missing run image mixins", func() {
			buildMixins := []string{"mixinA", "mixinB"}
			runMixins := []string{}

			err := stack.ValidateMixins("some/build", buildMixins, "some/run", runMixins)

			h.AssertError(t, err, "'some/run' missing required mixin(s): mixinA, mixinB")
		})

		it("returns an error with any invalid build image mixins", func() {
			buildMixins := []string{"run:mixinA", "run:mixinB"}
			runMixins := []string{}

			err := stack.ValidateMixins("some/build", buildMixins, "some/run", runMixins)

			h.AssertError(t, err, "'some/build' contains run-only mixin(s): run:mixinA, run:mixinB")
		})

		it("returns an error with any invalid run image mixins", func() {
			buildMixins := []string{}
			runMixins := []string{"build:mixinA", "build:mixinB"}

			err := stack.ValidateMixins("some/build", buildMixins, "some/run", runMixins)

			h.AssertError(t, err, "'some/run' contains build-only mixin(s): build:mixinA, build:mixinB")
		})
	})
}
