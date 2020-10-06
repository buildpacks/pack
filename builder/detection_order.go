package builder

import (
	"github.com/buildpacks/pack/internal/dist"
)

type DetectionOrderEntry struct {
	dist.BuildpackRef
	Cyclical            bool           `json:"cyclic,omitempty"`
	GroupDetectionOrder DetectionOrder `json:"nested_buildpacks,omitempty"`
}

type DetectionOrder []DetectionOrderEntry

const (
	OrderDetectionMaxDepth = -1
	OrderDetectionNone     = 0
)
