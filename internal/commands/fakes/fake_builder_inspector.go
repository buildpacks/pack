package fakes

import "github.com/buildpacks/pack"

type FakeBuilderInspector struct {
	InfoForLocal   *pack.BuilderInfo
	InfoForRemote  *pack.BuilderInfo
	ErrorForLocal  error
	ErrorForRemote error

	ReceivedForLocalName      string
	ReceivedForRemoteName     string
	CalculatedConfigForLocal  pack.BuilderInspectionConfig
	CalculatedConfigForRemote pack.BuilderInspectionConfig
}

func (i *FakeBuilderInspector) InspectBuilder(
	name string,
	daemon bool,
	modifiers ...pack.BuilderInspectionModifier,
) (*pack.BuilderInfo, error) {
	if daemon {
		i.CalculatedConfigForLocal = pack.BuilderInspectionConfig{}
		for _, mod := range modifiers {
			mod(&i.CalculatedConfigForLocal)
		}
		i.ReceivedForLocalName = name
		return i.InfoForLocal, i.ErrorForLocal
	}

	i.CalculatedConfigForRemote = pack.BuilderInspectionConfig{}
	for _, mod := range modifiers {
		mod(&i.CalculatedConfigForRemote)
	}
	i.ReceivedForRemoteName = name
	return i.InfoForRemote, i.ErrorForRemote
}
