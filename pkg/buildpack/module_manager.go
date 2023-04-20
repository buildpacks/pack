package buildpack

import (
	"github.com/buildpacks/pack/pkg/dist"
)

type ModuleManager struct {
	modules        []BuildModule
	flattenModules [][]BuildModule
	flatten        bool
	maxDepth       int
}

func NewModuleManager(flatten bool, maxDepth int) *ModuleManager {
	return &ModuleManager{
		flatten:  flatten,
		maxDepth: maxDepth,
	}
}

func (f *ModuleManager) Modules() []BuildModule {
	all := f.modules
	for _, modules := range f.flattenModules {
		all = append(all, modules...)
	}
	return all
}

func (f *ModuleManager) GetFlattenModules() [][]BuildModule {
	if f.flatten {
		return f.flattenModules
	}
	return nil
}

func (f *ModuleManager) AddModules(main BuildModule, deps ...BuildModule) {
	if !f.flatten {
		// default behavior
		f.modules = append(f.modules, append([]BuildModule{main}, deps...)...)
	} else {
		if f.maxDepth == -1 {
			// flatten all
			f.flattenModules = append(f.flattenModules, append([]BuildModule{main}, deps...))
		} else {
			recurser := newFlattenModuleRecurser(f.maxDepth)
			f.flattenModules = append(f.flattenModules, recurser.calculateFlattenModules(main, deps, 0)...)
		}
	}
}

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
		bps, newDeps := buildpacksFromGroup(orders, deps)
		if depth == f.maxDepth {
			modules = append(modules, append([]BuildModule{main}, bps...))
		}
		if depth < f.maxDepth {
			for _, bp := range bps {
				modules = append(modules, []BuildModule{main})
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
