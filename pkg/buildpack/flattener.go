package buildpack

import "io"

type BuildFlattenModule interface {
	Open() (io.ReadCloser, error)
	Descriptors() []Descriptor
}

type buildpacksFlattenerModule struct {
	descriptors []Descriptor
}

func NewBuildpacksFlattenerModule(buildmodules []BuildModule) BuildFlattenModule {
	var bpFlattenerModule buildpacksFlattenerModule

	for _, module := range buildmodules {
		bpFlattenerModule.descriptors = append(bpFlattenerModule.descriptors, module.Descriptor())
	}

	return bpFlattenerModule
}

func (bfm buildpacksFlattenerModule) Descriptors() []Descriptor {
	return bfm.descriptors
}

func (bfm buildpacksFlattenerModule) Open() (io.ReadCloser, error) {
	return nil, nil
}
