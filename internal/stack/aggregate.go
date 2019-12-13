package stack

import (
	"sort"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/stringset"
)

// Aggregate aggregates two set of stacks into a merged set of stacks where compatibility exists.
//
// Examples:
//
// 	stacksA = [{ID: "stack1", mixins: ["build:mixinA", "mixinB", "run:mixinC"]}}]
// 	stacksB = [{ID: "stack1", mixins: ["build:mixinA", "run:mixinC"]}}]
// 	result = [{ID: "stack1", mixins: ["build:mixinA", "mixinB", "run:mixinC"]}}]
//
// 	stacksA = [{ID: "stack1", mixins: ["build:mixinA"]}}, {ID: "stack2", mixins: ["mixinA"]}}]
// 	stacksB = [{ID: "stack1", mixins: ["run:mixinC"]}}, {ID: "stack2", mixins: ["mixinA"]}}]
// 	result = [{ID: "stack1", mixins: ["build:mixinA", "run:mixinC"]}}, {ID: "stack2", mixins: ["mixinA"]}}]
//
// 	stacksA = [{ID: "stack1", mixins: ["build:mixinA"]}}, {ID: "stack2", mixins: ["mixinA"]}}]
// 	stacksB = [{ID: "stack2", mixins: ["mixinA", "run:mixinB"]}}]
// 	result = [{ID: "stack2", mixins: ["mixinA", "run:mixinB"]}}]
//
// 	stacksA = [{ID: "stack1", mixins: ["build:mixinA"]}}]
// 	stacksB = [{ID: "stack2", mixins: ["mixinA", "run:mixinB"]}}]
// 	result = []
//
func Aggregate(stacksA []dist.Stack, stacksB []dist.Stack) []dist.Stack {
	set := map[string][]string{}

	for _, s := range stacksA {
		set[s.ID] = s.Mixins
	}

	var results []dist.Stack
	for _, s := range stacksB {
		if stackMixins, ok := set[s.ID]; ok {
			mixinsSet := stringset.FromSlice(append(stackMixins, s.Mixins...))
			var mixins []string
			for s := range mixinsSet {
				mixins = append(mixins, s)
			}
			sort.Strings(mixins)

			results = append(results, dist.Stack{
				ID:     s.ID,
				Mixins: mixins,
			})
		}
	}

	return results
}
