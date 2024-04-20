package stage

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options"

// VOLUMECommand implements packerfile.Packerfile.
func (s *Stage) VOLUMECommand(options.VOLUME, ...options.VOLUME) error {
	panic("unimplemented")
}
