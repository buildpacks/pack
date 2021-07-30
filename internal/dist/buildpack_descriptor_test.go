package dist_test

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpackDescriptor(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testBuildpackDescriptor", testBuildpackDescriptor, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackDescriptor(t *testing.T, when spec.G, it spec.S) {
	when("#EnsureStackSupport", func() {
		when("not validating against run image mixins", func() {
			it("ignores run-only mixins", func() {
				bp := dist.BuildpackDescriptor{
					Info: dist.BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []dist.Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC"}
				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, false))
			})

			it("works with wildcard stack", func() {
				bp := dist.BuildpackDescriptor{
					Info: dist.BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []dist.Stack{{
						ID:     "*",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC"}
				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, false))
			})

			it("returns an error with any missing (and non-ignored) mixins", func() {
				bp := dist.BuildpackDescriptor{
					Info: dist.BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []dist.Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinX", "mixinY", "run:mixinZ"},
					}},
				}

				providedMixins := []string{"mixinA", "mixinB"}
				err := bp.EnsureStackSupport("some.stack.id", providedMixins, false)

				h.AssertError(t, err, "buildpack 'some.buildpack.id@some.buildpack.version' requires missing mixin(s): mixinX, mixinY")
			})
		})

		when("validating against run image mixins", func() {
			it("requires run-only mixins", func() {
				bp := dist.BuildpackDescriptor{
					Info: dist.BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []dist.Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC", "run:mixinD"}

				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, true))
			})

			it("returns an error with any missing mixins", func() {
				bp := dist.BuildpackDescriptor{
					Info: dist.BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []dist.Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinX", "mixinY", "run:mixinZ"},
					}},
				}

				providedMixins := []string{"mixinA", "mixinB"}

				err := bp.EnsureStackSupport("some.stack.id", providedMixins, true)

				h.AssertError(t, err, "buildpack 'some.buildpack.id@some.buildpack.version' requires missing mixin(s): mixinX, mixinY, run:mixinZ")
			})
		})

		it("returns an error when buildpack does not support stack", func() {
			bp := dist.BuildpackDescriptor{
				Info: dist.BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []dist.Stack{{
					ID:     "some.stack.id",
					Mixins: []string{"mixinX", "mixinY"},
				}},
			}

			err := bp.EnsureStackSupport("some.nonexistent.stack.id", []string{"mixinA"}, true)

			h.AssertError(t, err, "buildpack 'some.buildpack.id@some.buildpack.version' does not support stack 'some.nonexistent.stack.id")
		})

		it("skips validating order buildpack", func() {
			bp := dist.BuildpackDescriptor{
				Info: dist.BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []dist.Stack{},
			}

			h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", []string{"mixinA"}, true))
		})
	})
}
