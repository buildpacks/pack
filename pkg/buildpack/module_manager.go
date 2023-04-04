package buildpack

type ModuleManager struct {
	modules        []BuildModule
	flattenModules [][]BuildModule
	Flatten        bool
}

func NewModuleManager(flatten bool) *ModuleManager {
	return &ModuleManager{
		Flatten: flatten,
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
	return f.flattenModules
}

func (f *ModuleManager) AddFlattenModules(modules []BuildModule) {
	f.flattenModules = append(f.flattenModules, modules)
}

func (f *ModuleManager) AddModules(main BuildModule, deps ...BuildModule) {
	modules := append([]BuildModule{main}, deps...)
	if f.Flatten && len(deps) > 0 {
		f.flattenModules = append(f.flattenModules, modules)
	} else {
		f.modules = append(f.modules, modules...)
	}
}

func (f *ModuleManager) IsFlatten(module BuildModule) bool {
	if f.Flatten {
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
