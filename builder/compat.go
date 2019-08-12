package builder

type v1OrderTOML struct {
	Groups []v1Group `toml:"groups"`
}

type v1Group struct {
	Buildpacks []v1BuildpackRef `toml:"buildpacks"`
}

type v1BuildpackRef struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Optional bool   `toml:"optional,omitempty"`
}

func v1OrderTOMLFromOrderTOML(order orderTOML) v1OrderTOML {
	var groups []v1Group
	for _, g := range order.Order {
		var bps []v1BuildpackRef
		for _, b := range g.Group {
			bps = append(bps, v1BuildpackRef{
				ID:       b.ID,
				Version:  b.Version,
				Optional: b.Optional,
			})
		}

		groups = append(groups, v1Group{
			Buildpacks: bps,
		})
	}

	return v1OrderTOML{
		Groups: groups,
	}
}
