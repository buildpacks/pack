package dist

import (
	"testing"

	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"
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
				bp := BuildpackDescriptor{
					Info: BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC"}
				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, false))
			})

			it("returns an error with any missing (and non-ignored) mixins", func() {
				bp := BuildpackDescriptor{
					Info: BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []Stack{{
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
				bp := BuildpackDescriptor{
					Info: BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC", "run:mixinD"}

				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, true))
			})

			it("returns an error with any missing mixins", func() {
				bp := BuildpackDescriptor{
					Info: BuildpackInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					Stacks: []Stack{{
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
			bp := BuildpackDescriptor{
				Info: BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []Stack{{
					ID:     "some.stack.id",
					Mixins: []string{"mixinX", "mixinY"},
				}},
			}

			err := bp.EnsureStackSupport("some.nonexistent.stack.id", []string{"mixinA"}, true)

			h.AssertError(t, err, "buildpack 'some.buildpack.id@some.buildpack.version' does not support stack 'some.nonexistent.stack.id")
		})

		it("skips validating order buildpack", func() {
			bp := BuildpackDescriptor{
				Info: BuildpackInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				Stacks: []Stack{},
			}

			h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", []string{"mixinA"}, true))
		})
	})
}
