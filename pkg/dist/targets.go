package dist

type Target struct {
	OS            string         `json:"os" toml:"os"`
	Arch          string         `json:"arch" toml:"arch"`
	ArchVariant   string         `json:"variant,omitempty" toml:"variant,omitempty"`
	Distributions []Distribution `json:"distros,omitempty" toml:"distros,omitempty"`
}

type Distribution struct {
	Name    string `json:"name,omitempty" toml:"name,omitempty"`
	Version string `json:"version,omitempty" toml:"version,omitempty"`
}

func (t Target) Range(loop func(target Target) error) error {
	for _, d := range t.Distributions {
		t.Distributions = []Distribution{d}
		if err := loop(t); err != nil {
			return err
		}
	}

	return nil
}

func (t Target) MultiArch() bool {
	return len(t.Distributions) > 1
}
