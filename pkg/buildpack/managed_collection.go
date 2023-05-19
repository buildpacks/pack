package buildpack

import (
	"github.com/buildpacks/pack/pkg/dist"
)

const (
	FlattenMaxDepth = -1
	FlattenNone     = 0
)

type ManagedCollection struct {
	explodedModules  []BuildModule
	flattenedModules [][]BuildModule
	excluded         map[string]struct{}
	flatten          bool
	maxDepth         int
}

func NewModuleManager(flatten bool, maxDepth int, exclude []string) *ManagedCollection {
	return &ManagedCollection{
		flatten:          flatten,
		maxDepth:         maxDepth,
		explodedModules:  []BuildModule{},
		flattenedModules: [][]BuildModule{},
		excluded:         Set(exclude),
	}
}

// AllModules returns all explodedModules handle by the manager
func (f *ManagedCollection) AllModules() []BuildModule {
	all := f.explodedModules
	for _, modules := range f.flattenedModules {
		all = append(all, modules...)
	}
	return all
}

// ExplodedModules returns all modules that will be added to the output artifact as a single layer containing a single module.
func (f *ManagedCollection) ExplodedModules() []BuildModule {
	return f.explodedModules
}

// FlattenedModules returns all modules that will be added to the output artifact as a single layer containing multiple modules.
func (f *ManagedCollection) FlattenedModules() [][]BuildModule {
	if f.flatten {
		return f.flattenedModules
	}
	return nil
}

// AddModules determines whether the explodedModules must be added as flattened or not. It uses
// flatten and maxDepth configuration given during initialization of the manager.
func (f *ManagedCollection) AddModules(main BuildModule, deps ...BuildModule) {
	if !f.flatten {
		// default behavior
		f.explodedModules = append(f.explodedModules, append([]BuildModule{main}, deps...)...)
	} else {
		if _, ok := f.excluded[main.Descriptor().Info().FullName()]; ok {
			f.explodesModules = append(f.explodesModules, append([]BuildModule{main}, deps...)...)
			f.setExcludedModules(deps)
			return
		}
		if f.maxDepth <= FlattenMaxDepth {
			excluded, newDeps := f.calculateExcludeModules(deps...)
			f.explodesModules = append(f.explodesModules, excluded...)
			// flatten all
			if len(f.flattenedModules) == 1 {
				f.flattenedModules[0] = append(f.flattenedModules[0], append([]BuildModule{main}, newDeps...)...)
			} else {
				f.flattenedModules = append(f.flattenedModules, append([]BuildModule{main}, newDeps...))
			}
		} else {
			recurser := newFlattenModuleRecurser(f.maxDepth)
			calculatedModules := recurser.calculateFlattenedModules(main, deps, 0)
			for _, modules := range calculatedModules {
				if len(modules) == 1 {
					f.explodedModules = append(f.explodedModules, modules...)
				} else {
					excluded, newModules := f.calculateExcludeModules(modules...)
					f.explodesModules = append(f.explodesModules, excluded...)
					f.flattenedModules = append(f.flattenedModules, newModules)
				}
			}
		}
	}
}

// ShouldFlatten returns true if the given module is flattened.
func (f *ManagedCollection) ShouldFlatten(module BuildModule) bool {
	if f.flatten {
		for _, modules := range f.flattenedModules {
			for _, v := range modules {
				if v == module {
					return true
				}
			}
		}
	}
	return false
}

// calculateExcludeModules separates the given modules into two groups: excluded and not excluded.
func (f *ManagedCollection) calculateExcludeModules(deps ...BuildModule) ([]BuildModule, []BuildModule) {
	if len(f.excluded) == 0 {
		return nil, deps
	}
	exclude := make([]BuildModule, 0)
	newDeps := make([]BuildModule, 0)

	// update excluded modules with dependencies from composite buildpacks
	for _, dep := range deps {
		if _, ok := f.excluded[dep.Descriptor().Info().FullName()]; ok {
			if len(dep.Descriptor().Order()) > 0 {
				updateSetFromGroups(f.excluded, dep.Descriptor().Order())
			}
		}
	}
	for _, dep := range deps {
		if _, ok := f.excluded[dep.Descriptor().Info().FullName()]; ok {
			exclude = append(exclude, dep)
		} else {
			newDeps = append(newDeps, dep)
		}
	}
	f.removeExcludedFromFlattenModules()
	return exclude, newDeps
}

// setExcludedModules adds the given modules to the excluded map and removes the given dependencies from the flattened
// modules if they were already added.
func (f *ManagedCollection) setExcludedModules(deps []BuildModule) {
	type void struct{}
	var member void
	for _, dep := range deps {
		if _, ok := f.excluded[dep.Descriptor().Info().FullName()]; !ok {
			f.excluded[dep.Descriptor().Info().FullName()] = member
		}
	}
	f.removeExcludedFromFlattenModules()
}

func (f *ManagedCollection) removeExcludedFromFlattenModules() {
	for i, modules := range f.flattenedModules {
		j := 0
		for _, m := range modules {
			if _, ok := f.excluded[m.Descriptor().Info().FullName()]; !ok {
				modules[j] = m
				j++
			}
		}
		f.flattenedModules[i] = modules[:j]
	}
}

type flattenModuleRecurser struct {
	maxDepth int
}

func newFlattenModuleRecurser(maxDepth int) *flattenModuleRecurser {
	return &flattenModuleRecurser{
		maxDepth: maxDepth,
	}
}

// calculateFlattenedModules returns groups of modules that will be added to the output artifact as a single layer containing multiple modules.
// It takes the given main module and its dependencies and based on the depth it will recursively calculate the groups of modules inspecting if the main
// module is a composited Buildpack or not until it reaches the maxDepth.
func (f *flattenModuleRecurser) calculateFlattenedModules(main BuildModule, deps []BuildModule, depth int) [][]BuildModule {
	modules := make([][]BuildModule, 0)
	groups := main.Descriptor().Order()
	if len(groups) > 0 {
		if depth == f.maxDepth {
			modules = append(modules, append([]BuildModule{main}, deps...))
		}
		if depth < f.maxDepth {
			nextBPs, nextDeps := buildpacksFromGroups(groups, deps)
			modules = append(modules, []BuildModule{main})
			for _, bp := range nextBPs {
				modules = append(modules, f.calculateFlattenedModules(bp, nextDeps, depth+1)...)
			}
		}
	} else {
		// It is not a composited Buildpack, we add it as a single module
		modules = append(modules, []BuildModule{main})
	}
	return modules
}

// buildpacksFromGroups split the given dependencies into two groups: main buildpacks with those that belongs to the given group and
// the rest of the dependencies.
func buildpacksFromGroups(order dist.Order, deps []BuildModule) ([]BuildModule, []BuildModule) {
	bps := make([]BuildModule, 0)
	newDeps := make([]BuildModule, 0)

	set := make(map[string]struct{})
	updateSetFromGroups(set, order)
	for _, dep := range deps {
		if _, ok := set[dep.Descriptor().Info().FullName()]; ok {
			bps = append(bps, dep)
		} else {
			newDeps = append(newDeps, dep)
		}
	}

	return bps, newDeps
}

// updateSetFromGroups adds the buildpacks FullName() from the Order to the given set
func updateSetFromGroups(set map[string]struct{}, order dist.Order) {
	type void struct{}
	var member void

	for _, groups := range order {
		for _, group := range groups.Group {
			if _, ok := set[group.FullName()]; !ok {
				set[group.FullName()] = member
			}
		}
	}
}
