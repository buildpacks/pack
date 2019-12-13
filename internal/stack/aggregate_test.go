package stack_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/stack"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestAggregate(t *testing.T) {
	spec.Run(t, "testMixinValidation", testAggregate, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAggregate(t *testing.T, when spec.G, it spec.S) {
	when("a stack has more mixins than the other", func() {
		it("add mixins", func() {
			result := stack.Aggregate(
				[]dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "mixinB", "run:mixinC"}}},
				[]dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "run:mixinC"}}},
			)

			h.AssertEq(t, len(result), 1)
			h.AssertEq(t, result, []dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "mixinB", "run:mixinC"}}})
		})
	})

	when("stacks don't match id", func() {
		it("returns no stacks", func() {
			result := stack.Aggregate(
				[]dist.Stack{{ID: "stack1", Mixins: []string{"build:mixinA", "mixinB", "run:mixinC"}}},
				[]dist.Stack{{ID: "stack2", Mixins: []string{"build:mixinA", "run:mixinC"}}},
			)

			h.AssertEq(t, len(result), 0)
		})
	})

	when("a set of stacks has extra stacks", func() {
		it("removes extra stacks", func() {
			result := stack.Aggregate(
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
}
