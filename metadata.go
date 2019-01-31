package pack

const (
	StackLabel    = "io.buildpacks.stack.id"
	MetadataLabel = "io.buildpacks.pack.metadata"
)

type BuilderImageMetadata struct {
	RunImage BuilderRunImageMetadata `json:"runImage"`
}

type BuilderRunImageMetadata struct {
	Image   string   `json:"image"`
	Mirrors []string `json:"mirrors"`
}
