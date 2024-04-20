package stage

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options"

// STOPSIGNALCommand implements packerfile.Packerfile.
func (s *Stage) STOPSIGNALCommand(options.STOPSIGNAL) error {
	panic("unimplemented")
}
