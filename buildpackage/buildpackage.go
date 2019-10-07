package buildpackage

import "github.com/buildpack/pack/dist"

const MetadataLabel = "io.buildpacks.buildpackage.metadata"

type Config struct {
	Default dist.BuildpackInfo `toml:"default"`
	Blobs   []dist.BlobConfig  `toml:"blobs"`
	Stacks  []dist.Stack       `toml:"stacks"`
}

type Metadata struct {
	dist.BuildpackInfo
	Stacks []dist.Stack `toml:"stacks" json:"stacks"`
}
