package flatten

import "github.com/buildpacks/pack/pkg/buildpack"

type Modules struct {
	flatten [][]buildpack.BuildModule
}

func (f *Modules) GetFlattenModules() [][]buildpack.BuildModule {
	return f.flatten
}

func (f *Modules) AddFlattenModules(modules []buildpack.BuildModule) {
	f.flatten = append(f.flatten, modules[:])
}

func (f *Modules) Flatten(module buildpack.BuildModule) bool {
	for _, modules := range f.flatten {
		for _, v := range modules {
			if v == module {
				return true
			}
		}
	}
	return false
}
