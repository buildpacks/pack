package buildpack

type ManagedCollection struct {
	explodedModules  []BuildModule
	flattenedModules []BuildModule
	flatten          bool
}

func NewModuleManager(flatten bool) *ManagedCollection {
	return &ManagedCollection{
		flatten:          flatten,
		explodedModules:  []BuildModule{},
		flattenedModules: []BuildModule{},
	}
}

// AllModules returns all explodedModules handle by the manager
func (f *ManagedCollection) AllModules() []BuildModule {
	all := f.explodedModules
	all = append(f.explodedModules, f.flattenedModules...)
	return all
}

// ExplodedModules returns all modules that will be added to the output artifact as a single layer containing a single module.
func (f *ManagedCollection) ExplodedModules() []BuildModule {
	return f.explodedModules
}

// FlattenedModules returns all modules that will be added to the output artifact as a single layer containing multiple modules.
func (f *ManagedCollection) FlattenedModules() [][]BuildModule {
	if f.flatten {
		modules := [][]BuildModule{}
		modules = append(modules, f.flattenedModules)
		return modules
	}
	return nil
}

// AddModules determines whether the explodedModules must be added as flattened or not.
func (f *ManagedCollection) AddModules(main BuildModule, deps ...BuildModule) {
	if !f.flatten {
		// default behavior
		f.explodedModules = append(f.explodedModules, append([]BuildModule{main}, deps...)...)
	} else {
		// flatten all
		f.flattenedModules = append(f.flattenedModules, append([]BuildModule{main}, deps...)...)
	}
}

// ShouldFlatten returns true if the given module is flattened.
func (f *ManagedCollection) ShouldFlatten(module BuildModule) bool {
	if f.flatten {
		for _, v := range f.flattenedModules {
			if v == module {
				return true
			}
		}
	}
	return false
}
