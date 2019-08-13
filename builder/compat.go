package builder

type V1Order []V1Group

type V1Group struct {
	Buildpacks []BuildpackRef `toml:"buildpacks" json:"buildpacks"`
}

type v1OrderTOML struct {
	Groups []V1Group `toml:"groups" json:"groups"`
}

func (o Order) ToV1Order() V1Order {
	var order V1Order
	for _, gp := range o {
		var buildpacks []BuildpackRef
		for _, bp := range gp.Group {
			buildpacks = append(buildpacks, bp)
		}

		order = append(order, V1Group{
			Buildpacks: buildpacks,
		})
	}

	return order
}
