package stage

import (
	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	"github.com/buildpacks/pack/internal/buildkit/state"
)

var _ packerfile.Packerfile = (*Stage)(nil)

// [Stage] exposes a set of methods to instruct the way an Image or an ImageIndex should be build.
//
// A [Stage] can be Either of a Single Architecure or of Multi Architecture.
//
// This can be either a _buildtime stage_ or an **runtime stage**.
// The [state/State] package is wrapped inside Stage.
type Stage struct {
	state  state.State
	digest string
}
