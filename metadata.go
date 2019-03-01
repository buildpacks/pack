package pack

const (
	StackLabel           = "io.buildpacks.stack.id"
	BuilderMetadataLabel = "io.buildpacks.builder.metadata"
)

type BuilderImageMetadata struct {
	RunImage   BuilderRunImageMetadata     `json:"runImage"`
	Buildpacks []BuilderBuildpacksMetadata `json:"buildpacks"`
}

type BuilderRunImageMetadata struct {
	Image   string   `json:"image"`
	Mirrors []string `json:"mirrors"`
}

type BuilderBuildpacksMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}
