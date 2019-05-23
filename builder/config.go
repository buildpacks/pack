package builder

type Config struct {
	Description string            `toml:"description"`
	Buildpacks  []BuildpackConfig `toml:"buildpacks"`
	Groups      []GroupMetadata   `toml:"groups"`
	Stack       StackConfig       `toml:"stack"`
	Lifecycle   LifecycleConfig   `toml:"lifecycle"`
}

type BuildpackConfig struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	URI     string `toml:"uri"`
	Latest  bool   `toml:"latest"`
}

type StackConfig struct {
	ID              string   `toml:"id"`
	BuildImage      string   `toml:"build-image"`
	RunImage        string   `toml:"run-image"`
	RunImageMirrors []string `toml:"run-image-mirrors,omitempty"`
}

type LifecycleConfig struct {
	URI     string `toml:"uri"`
	Version string `toml:"version"`
}
