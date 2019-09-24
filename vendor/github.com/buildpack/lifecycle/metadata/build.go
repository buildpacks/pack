package metadata

const BuildMetadataLabel = "io.buildpacks.build.metadata"

type BuildMetadata struct {
	BOM        interface{}         `json:"bom"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	Launcher   LauncherMetadata    `json:"launcher"`
}

type BuildpackMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type LauncherMetadata struct {
	Version string         `json:"version"`
	Source  SourceMetadata `json:"source"`
}

type SourceMetadata struct {
	Git GitMetadata `json:"git"`
}

type GitMetadata struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}
