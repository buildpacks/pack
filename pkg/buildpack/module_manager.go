package buildpack

type ModuleManager struct {
	modules        []BuildModule
	flattenModules [][]BuildModule
	flatten        bool
}

func NewModuleManager(flatten bool) *ModuleManager {
	return &ModuleManager{
		flatten: flatten,
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
	modules := append([]BuildModule{main}, deps...)
	if f.flatten && len(deps) > 0 {
		f.flattenModules = append(f.flattenModules, modules)
	} else {
		f.modules = append(f.modules, modules...)
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
