package buildpack

type Flattener interface {
	FlatBuildpacks()
}

type BuildpacksFlattener struct {
}

func NewBuildpacksFlattener() BuildpacksFlattener {
	return BuildpacksFlattener{}
}

func (bp BuildpacksFlattener) FlatBuildpacks(bps []BuildModule) []BuildModule {
	return bps //TODO: Implementation
}
