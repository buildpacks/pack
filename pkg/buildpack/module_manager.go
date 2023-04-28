package buildpack

import (
	"github.com/buildpacks/pack/pkg/dist"
)

const (
	FlattenMaxDepth = -1
	FlattenNone     = 0
)

type ModuleManager struct {
	modules        []BuildModule
	flattenModules [][]BuildModule
	flatten        bool
	maxDepth       int
}

func NewModuleManager(flatten bool, maxDepth int) *ModuleManager {
	return &ModuleManager{
		flatten:        flatten,
		maxDepth:       maxDepth,
		modules:        []BuildModule{},
		flattenModules: [][]BuildModule{},
	}
}

// AllModules returns all modules handle by the manager
func (f *ModuleManager) AllModules() []BuildModule {
	all := f.modules
	for _, modules := range f.flattenModules {
		all = append(all, modules...)
	}
	return all
}

// NonFlattenModules returns all none flatten modules handle by the manager
func (f *ModuleManager) NonFlattenModules() []BuildModule {
	return f.modules
}

// FlattenModules returns all flatten modules handle by the manager.
func (f *ModuleManager) FlattenModules() [][]BuildModule {
	if f.flatten {
		return f.flattenModules
	}
	return nil
}

// AddModules determines whether the modules must be added as flatten or not. It uses
// flatten and maxDepth configuration given during initialization of the manager.
func (f *ModuleManager) AddModules(main BuildModule, deps ...BuildModule) {
	if !f.flatten {
		// default behavior
		f.modules = append(f.modules, append([]BuildModule{main}, deps...)...)
	} else {
		if f.maxDepth <= FlattenMaxDepth {
			// flatten all
			if len(f.flattenModules) == 1 {
				f.flattenModules[0] = append(f.flattenModules[0], append([]BuildModule{main}, deps...)...)
			} else {
				f.flattenModules = append(f.flattenModules, append([]BuildModule{main}, deps...))
			}
		} else {
			recurser := newFlattenModuleRecurser(f.maxDepth)
			calculateModules := recurser.calculateFlattenModules(main, deps, 0)
			for _, modules := range calculateModules {
				if len(modules) == 1 {
					f.modules = append(f.modules, modules...)
				} else {
					f.flattenModules = append(f.flattenModules, modules)
				}
			}
		}
	}
}

// IsFlatten returns true if the given module is flatten.
func (f *ModuleManager) IsFlatten(module BuildModule) bool {
	if f.flatten {
		for _, modules := range f.flattenModules {
			for _, v := range modules {
				if v == module {
					return true
				}
			}
		}
	}
	return false
}

type flattenModuleRecurser struct {
	maxDepth int
}

func newFlattenModuleRecurser(maxDepth int) *flattenModuleRecurser {
	return &flattenModuleRecurser{
		maxDepth: maxDepth,
	}
}

func (f *flattenModuleRecurser) calculateFlattenModules(main BuildModule, deps []BuildModule, depth int) [][]BuildModule {
	modules := make([][]BuildModule, 0)
	orders := main.Descriptor().Order()
	if len(orders) > 0 {
		if depth == f.maxDepth {
			modules = append(modules, append([]BuildModule{main}, deps...))
		}
		if depth < f.maxDepth {
			bps, newDeps := buildpacksFromGroup(orders, deps)
			modules = append(modules, []BuildModule{main})
			for _, bp := range bps {
				modules = append(modules, f.calculateFlattenModules(bp, newDeps, depth+1)...)
			}
		}
	} else {
		modules = append(modules, []BuildModule{main})
	}
	return modules
}

func buildpacksFromGroup(orders dist.Order, deps []BuildModule) ([]BuildModule, []BuildModule) {
	bps := make([]BuildModule, 0)
	newDeps := make([]BuildModule, 0)

	type void struct{}
	var member void
	set := make(map[string]void)
	for _, order := range orders {
		for _, group := range order.Group {
			set[group.FullName()] = member
		}
	}

	for _, dep := range deps {
		if _, ok := set[dep.Descriptor().Info().FullName()]; ok {
			bps = append(bps, dep)
		} else {
			newDeps = append(newDeps, dep)
		}
	}

	return bps, newDeps
}
