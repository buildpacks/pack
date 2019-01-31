package pack

const (
	StackLabel           = "io.buildpacks.stack.id"
	BuilderMetadataLabel = "io.buildpacks.builder.metadata"
)

type BuilderImageMetadata struct {
	RunImage BuilderRunImageMetadata `json:"runImage"`
}

type BuilderRunImageMetadata struct {
	Image   string   `json:"image"`
	Mirrors []string `json:"mirrors"`
}
