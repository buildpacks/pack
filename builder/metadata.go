package builder

import (
	"github.com/buildpack/pack/buildpack"

	"github.com/buildpack/pack/lifecycle"
)

const MetadataLabel = "io.buildpacks.builder.metadata"

type Metadata struct {
	Description string              `json:"description"`
	Buildpacks  []BuildpackMetadata `json:"buildpacks"`
	Groups      V1Order             `json:"groups"` // deprecated
	Stack       StackMetadata       `json:"stack"`
	Lifecycle   lifecycle.Metadata  `json:"lifecycle"`
}

type BuildpackMetadata struct {
	buildpack.BuildpackInfo
	Latest bool `json:"latest"` // deprecated
}

type StackMetadata struct {
	RunImage RunImageMetadata `json:"runImage" toml:"run-image"`
}

type RunImageMetadata struct {
	Image   string   `json:"image" toml:"image"`
	Mirrors []string `json:"mirrors" toml:"mirrors"`
}

func processMetadata(md *Metadata) error {
	for i, bp := range md.Buildpacks {
		var matchingBps []buildpack.BuildpackInfo
		for _, bp2 := range md.Buildpacks {
			if bp.ID == bp2.ID {
				matchingBps = append(matchingBps, bp.BuildpackInfo)
			}
		}

		if len(matchingBps) == 1 {
			md.Buildpacks[i].Latest = true
		}
	}

	return nil
}
