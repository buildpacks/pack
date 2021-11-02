package stack_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/pkg/dist"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestMerge(t *testing.T) {
	spec.Run(t, "testMerge", testMerge, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testMerge(t *testing.T, when spec.G, it spec.S) {
	when("MergeCompatible", func() {
		when("a stack has more mixins than the other", func() {
			it("add mixins", func() {
				result := stack.MergeCompatible(
					[]dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "mixinB", "run:mixinC"}}},
					[]dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "run:mixinC"}}},
				)

				h.AssertEq(t, len(result), 1)
				h.AssertEq(t, result, []dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "mixinB", "run:mixinC"}}})
			})
		})

		when("stacks don't match id", func() {
			it("returns no stacks", func() {
				result := stack.MergeCompatible(
					[]dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "mixinB", "run:mixinC"}}},
					[]dist.Stack{{ID: "stack2", Mixins: []string{"build:mixinA", "run:mixinC"}}},
				)

				h.AssertEq(t, len(result), 0)
			})
		})

		when("a set of stacks has extra stacks", func() {
			it("removes extra stacks", func() {
				result := stack.MergeCompatible(
					[]dist.Stack{{ID: "stack1"}},
					[]dist.Stack{
						{ID: "stack1"},
						{ID: "stack2"},
					},
				)

				h.AssertEq(t, len(result), 1)
				h.AssertEq(t, result, []dist.Stack{{ID: "stack1"}})
			})
		})
	})
}
