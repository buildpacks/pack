package stage

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options"

// COPYCommand implements packerfile.Packerfile.
func (s *Stage) COPYCommand(src string, desc []string, ops options.COPY) error {
	panic("unimplemented")
}
