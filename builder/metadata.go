package builder

import (
	"fmt"

	"github.com/buildpack/pack/buildpack"

	"github.com/buildpack/pack/lifecycle"
	"github.com/buildpack/pack/style"
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

func bpsWithID(metadata Metadata, id string) []BuildpackMetadata {
	var matchingBps []BuildpackMetadata
	for _, bp := range metadata.Buildpacks {
		if id == bp.ID {
			matchingBps = append(matchingBps, bp)
		}
	}
	return matchingBps
}

func hasBPWithVersion(bps []BuildpackMetadata, version string) bool {
	for _, bp := range bps {
		if bp.Version == version {
			return true
		}
	}
	return false
}

func processMetadata(md *Metadata) error {
	for i, bp := range md.Buildpacks {
		if len(bpsWithID(*md, bp.ID)) == 1 {
			md.Buildpacks[i].Latest = true
		}
	}

	for _, g := range md.Groups {
		for i := range g.Buildpacks {
			bpRef := &g.Buildpacks[i]
			bps := bpsWithID(*md, bpRef.ID)

			if len(bps) == 0 {
				return fmt.Errorf("no versions of buildpack %s were found on the builder", style.Symbol(bpRef.ID))
			}

			if bpRef.Version == "" {
				if len(bps) > 1 {
					return fmt.Errorf("unable to resolve version: multiple versions of %s - must specify an explicit version", style.Symbol(bpRef.ID))
				}

				bpRef.Version = bps[0].Version
			}

			if !hasBPWithVersion(bps, bpRef.Version) {
				return fmt.Errorf("buildpack %s with version %s was not found on the builder", style.Symbol(bpRef.ID), style.Symbol(bpRef.Version))
			}
		}
	}

	return nil
}
