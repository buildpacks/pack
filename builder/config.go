package builder

type Config struct {
	Buildpacks []BuildpackConfig `toml:"buildpacks"`
	Groups     []GroupMetadata   `toml:"groups"`
	Stack      StackConfig
}

type BuildpackConfig struct {
	ID     string `toml:"id"`
	URI    string `toml:"uri"`
	Latest bool   `toml:"latest"`
}

type StackConfig struct {
	ID              string   `toml:"id"`
	BuildImage      string   `toml:"build-image"`
	RunImage        string   `toml:"run-image"`
	RunImageMirrors []string `toml:"run-image-mirrors,omitempty"`
}
