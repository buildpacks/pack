package buildpack

// ManagedCollection defines the required behavior to deal with build modules when adding then to an OCI image.
type ManagedCollection interface {
	// AllModules returns all build modules handle by the manager
	AllModules() []BuildModule

	// ExplodedModules returns all build modules that will be added to the output artifact as a single layer
	// containing a single module.
	ExplodedModules() []BuildModule

	// AddModules determines whether the explodedModules must be added as flattened or not.
	AddModules(main BuildModule, deps ...BuildModule)

	// FlattenedModules returns all build modules that will be added to the output artifact as a single layer
	// containing multiple modules.
	FlattenedModules() [][]BuildModule

	// ShouldFlatten returns true if the given module should be flattened.
	ShouldFlatten(module BuildModule) bool
}

type managedCollection struct {
	explodedModules  []BuildModule
	flattenedModules [][]BuildModule
}

func (f *managedCollection) ExplodedModules() []BuildModule {
	return f.explodedModules
}

func (f *managedCollection) FlattenedModules() [][]BuildModule {
	return f.flattenedModules
}

func (f *managedCollection) AllModules() []BuildModule {
	all := f.explodedModules
	for _, modules := range f.flattenedModules {
		all = append(all, modules...)
	}
	return all
}

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
	flattenAll bool
}

func NewManagedCollectionV1(flattenAll bool) ManagedCollection {
	return &managedCollectionV1{
		flattenAll: flattenAll,
		managedCollection: managedCollection{
			explodedModules:  []BuildModule{},
			flattenedModules: [][]BuildModule{},
		},
	}
}

func (f *managedCollectionV1) AddModules(main BuildModule, deps ...BuildModule) {
	if !f.flattenAll {
		// default behavior
		f.explodedModules = append(f.explodedModules, append([]BuildModule{main}, deps...)...)
	} else {
		// flatten all
		f.flattenedModules = append(f.flattenedModules, append([]BuildModule{main}, deps...)...)
	}
}

func NewManagedCollectionV2(modules FlattenModuleInfos) ManagedCollection {
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

// managedCollectionV2 can be used when the build modules to be flattened are known at the point of initialization.
// The flattened build modules are provided when the collection is initialized and the collection will take care of
// keeping them in the correct group (flattened or exploded) once they are added.
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
			pos := ff.flattenedLayerFor(module)
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

// flattenedLayerFor given a module it will try to determine to which row (group) this module must be added to in order to
// be flattened. If it is not found, it means, the module must no me flattened at all
func (ff *managedCollectionV2) flattenedLayerFor(module BuildModule) int {
	// flattenModuleInfos to be flattened are representing a two-dimension array. where each row represents a group of
	// flattenModuleInfos that must be flattened together in the same layer.
	for i, flattenGroup := range ff.flattenGroups() {
		for _, buildModuleInfo := range flattenGroup.BuildModule() {
			if buildModuleInfo.FullName() == module.Descriptor().Info().FullName() {
				return i
			}
		}
	}
	return -1
}
