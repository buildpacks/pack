package stage

import (
	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
	"github.com/buildpacks/pack/internal/buildkit/packerfile/options/types"
)

// CMDCommand implements packerfile.Packerfile.
func (s *Stage) CMDCommand(cmd []string, ops options.CMD) error {
	s.state = s.state.AddCMD(cmd, ops.Form == types.SHELL)
	return nil
}
