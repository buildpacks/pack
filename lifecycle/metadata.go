package lifecycle

import (
	"github.com/Masterminds/semver"
)

type Metadata struct {
	Version *semver.Version `json:"version"`
	Path    string          `json:"-"`
}
