package buildpack

type FlattenModules struct {
	flatten [][]BuildModule
}

func (f *FlattenModules) GetFlattenModules() [][]BuildModule {
	return f.flatten
}

func (f *FlattenModules) AddFlattenModules(modules []BuildModule) {
	f.flatten = append(f.flatten, modules)
}

func (f *FlattenModules) Flatten(module BuildModule) bool {
	for _, modules := range f.flatten {
		for _, v := range modules {
			if v == module {
				return true
			}
		}
	}
	return false
}
