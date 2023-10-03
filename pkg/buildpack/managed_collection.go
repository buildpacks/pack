package buildpack

// ManagedCollection defines the required behavior to deal with BuildModule when adding then to an OCI image.
type ManagedCollection interface {
	AllModules() []BuildModule
	ExplodedModules() []BuildModule
	AddModules(main BuildModule, deps ...BuildModule)
	FlattenedModules() [][]BuildModule
	ShouldFlatten(module BuildModule) bool
}

type managedCollection struct {
	explodedModules  []BuildModule
	flattenedModules [][]BuildModule
}

// ExplodedModules returns all flattenModuleInfos that will be added to the output artifact as a single layer containing a single module.
func (f *managedCollection) ExplodedModules() []BuildModule {
	return f.explodedModules
}

// FlattenedModules returns all flattenModuleInfos that will be added to the output artifact as a single layer containing multiple flattenModuleInfos.
func (f *managedCollection) FlattenedModules() [][]BuildModule {
	return f.flattenedModules
}

// AllModules returns all explodedModules handle by the manager
func (f *managedCollection) AllModules() []BuildModule {
	all := f.explodedModules
	for _, modules := range f.flattenedModules {
		all = append(all, modules...)
	}
	return all
}

// ShouldFlatten returns true if the given module is flattened.
func (f *managedCollection) ShouldFlatten(module BuildModule) bool {
	for _, modules := range f.flattenedModules {
		for _, v := range modules {
			if v == module {
				return true
			}
		}
	}
	return false
}

// managedCollectionV1 can be used to flatten all the flattenModuleInfos or none of them.
type managedCollectionV1 struct {
	managedCollection
	flatten bool
}

func NewModuleManager(flatten bool) ManagedCollection {
	return &managedCollectionV1{
		flatten: flatten,
		managedCollection: managedCollection{
			explodedModules:  []BuildModule{},
			flattenedModules: [][]BuildModule{},
		},
	}
}

// AddModules determines whether the explodedModules must be added as flattened or not.
func (f *managedCollectionV1) AddModules(main BuildModule, deps ...BuildModule) {
	if !f.flatten {
		// default behavior
		f.explodedModules = append(f.explodedModules, append([]BuildModule{main}, deps...)...)
	} else {
		// flatten all
		if len(f.flattenedModules) == 1 {
			f.flattenedModules[0] = append(f.flattenedModules[0], append([]BuildModule{main}, deps...)...)
		} else {
			f.flattenedModules = append(f.flattenedModules, append([]BuildModule{main}, deps...))
		}
	}
}

func NewModuleManagerV2(modules FlattenModuleInfos) ManagedCollection {
	flattenGroups := 0
	if modules != nil {
		flattenGroups = len(modules.FlattenModules())
	}

	return &managedCollectionV2{
		flattenModuleInfos: modules,
		managedCollection: managedCollection{
			explodedModules:  []BuildModule{},
			flattenedModules: make([][]BuildModule, flattenGroups),
		},
	}
}

// managedCollectionV2 can be used when flattenModuleInfos to be flattened are known beforehand. These flattenModuleInfos are provided during
// initialization and the collection will take care of keeping them in the correct group once they are added.
type managedCollectionV2 struct {
	managedCollection
	flattenModuleInfos FlattenModuleInfos
}

func (ff *managedCollectionV2) flattenGroups() []ModuleInfos {
	return ff.flattenModuleInfos.FlattenModules()
}

func (ff *managedCollectionV2) AddModules(main BuildModule, deps ...BuildModule) {
	var allModules []BuildModule
	allModules = append(allModules, append([]BuildModule{main}, deps...)...)
	for _, module := range allModules {
		if ff.flattenModuleInfos != nil && len(ff.flattenGroups()) > 0 {
			pos := ff.flattenGroup(module)
			if pos >= 0 {
				ff.flattenedModules[pos] = append(ff.flattenedModules[pos], module)
			} else {
				// this module must not be flattened
				ff.explodedModules = append(ff.explodedModules, module)
			}
		} else {
			// we don't want to flatten anything
			ff.explodedModules = append(ff.explodedModules, module)
		}
	}
}

// flattenGroup given a module it will try to determine to which row (group) this module must be added to in order to
// be flattened. If it is not found, it means, the module must no me flattened at all
func (ff *managedCollectionV2) flattenGroup(module BuildModule) int {
	pos := -1
	// flattenModuleInfos to be flattened are representing a two-dimension array. where each row represents a group of
	// flattenModuleInfos that must be flattened together in the same layer.
init:
	for i, flattenGroup := range ff.flattenGroups() {
		for _, buildModuleInfo := range flattenGroup.BuildModule() {
			if buildModuleInfo.FullName() == module.Descriptor().Info().FullName() {
				pos = i
				break init
			}
		}
	}
	return pos
}
