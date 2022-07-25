package dist_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/heroku/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestBuildpackDescriptor(t *testing.T) {
	color.Disable(true)
	defer color.Disable(false)
	spec.Run(t, "testBuildpackDescriptor", testBuildpackDescriptor, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuildpackDescriptor(t *testing.T, when spec.G, it spec.S) {
	when("#EscapedID", func() {
		it("returns escaped ID", func() {
			bpDesc := dist.BuildpackDescriptor{
				WithInfo: dist.ModuleInfo{ID: "some/id"},
			}
			h.AssertEq(t, bpDesc.EscapedID(), "some_id")
		})
	})

	when("#EnsureStackSupport", func() {
		when("not validating against run image mixins", func() {
			it("ignores run-only mixins", func() {
				bp := dist.BuildpackDescriptor{
					WithInfo: dist.ModuleInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					WithStacks: []dist.Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC"}
				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, false))
			})

			it("works with wildcard stack", func() {
				bp := dist.BuildpackDescriptor{
					WithInfo: dist.ModuleInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					WithStacks: []dist.Stack{{
						ID:     "*",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC"}
				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, false))
			})

			it("returns an error with any missing (and non-ignored) mixins", func() {
				bp := dist.BuildpackDescriptor{
					WithInfo: dist.ModuleInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					WithStacks: []dist.Stack{{
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
					WithInfo: dist.ModuleInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					WithStacks: []dist.Stack{{
						ID:     "some.stack.id",
						Mixins: []string{"mixinA", "build:mixinB", "run:mixinD"},
					}},
				}

				providedMixins := []string{"mixinA", "build:mixinB", "mixinC", "run:mixinD"}

				h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", providedMixins, true))
			})

			it("returns an error with any missing mixins", func() {
				bp := dist.BuildpackDescriptor{
					WithInfo: dist.ModuleInfo{
						ID:      "some.buildpack.id",
						Version: "some.buildpack.version",
					},
					WithStacks: []dist.Stack{{
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
				WithInfo: dist.ModuleInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				WithStacks: []dist.Stack{{
					ID:     "some.stack.id",
					Mixins: []string{"mixinX", "mixinY"},
				}},
			}

			err := bp.EnsureStackSupport("some.nonexistent.stack.id", []string{"mixinA"}, true)

			h.AssertError(t, err, "buildpack 'some.buildpack.id@some.buildpack.version' does not support stack 'some.nonexistent.stack.id")
		})

		it("skips validating order buildpack", func() {
			bp := dist.BuildpackDescriptor{
				WithInfo: dist.ModuleInfo{
					ID:      "some.buildpack.id",
					Version: "some.buildpack.version",
				},
				WithStacks: []dist.Stack{},
			}

			h.AssertNil(t, bp.EnsureStackSupport("some.stack.id", []string{"mixinA"}, true))
		})
	})

	when("#Kind", func() {
		it("returns 'buildpack'", func() {
			bpDesc := dist.BuildpackDescriptor{}
			h.AssertEq(t, bpDesc.Kind(), "buildpack")
		})
	})

	when("#API", func() {
		it("returns the api", func() {
			bpDesc := dist.BuildpackDescriptor{
				WithAPI: api.MustParse("0.99"),
			}
			h.AssertEq(t, bpDesc.API().String(), "0.99")
		})
	})

	when("#Info", func() {
		it("returns the module info", func() {
			info := dist.ModuleInfo{
				ID:      "some-id",
				Name:    "some-name",
				Version: "some-version",
			}
			bpDesc := dist.BuildpackDescriptor{
				WithInfo: info,
			}
			h.AssertEq(t, bpDesc.Info(), info)
		})
	})

	when("#Order", func() {
		it("returns the order", func() {
			order := dist.Order{
				dist.OrderEntry{Group: []dist.ModuleRef{
					{ModuleInfo: dist.ModuleInfo{
						ID: "some-id", Name: "some-name", Version: "some-version",
					}},
				}},
			}
			bpDesc := dist.BuildpackDescriptor{
				WithOrder: order,
			}
			h.AssertEq(t, bpDesc.Order(), order)
		})
	})

	when("#Stacks", func() {
		it("returns the stacks", func() {
			stacks := []dist.Stack{
				{ID: "some-id", Mixins: []string{"some-mixin"}},
			}
			bpDesc := dist.BuildpackDescriptor{
				WithStacks: stacks,
			}
			h.AssertEq(t, bpDesc.Stacks(), stacks)
		})
	})
}
