package pack

const (
	StackLabel    = "io.buildpacks.stack.id"
	MetadataLabel = "io.buildpacks.pack.metadata"
)

type BuilderImageMetadata struct {
	RunImages []string `json:"runImages"`
}
