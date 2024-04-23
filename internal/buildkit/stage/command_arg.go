package stage

import (
	"fmt"

	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
)

// ARGCommand implements packerfile.Packerfile.
func (s *Stage) ARGCommand(ops options.ARG) error {
	state := s.state.AddArgs(fmt.Sprintf("%s=%s", ops.Key, ops.Value))
	s.state.State = state.State
	return nil
}
