package stack

type Metadata struct {
	RunImage RunImageMetadata `toml:"run-image" json:"runImage"`
}

type RunImageMetadata struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors"`
}
