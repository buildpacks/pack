package stage

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options"

// EXPOSECommand implements packerfile.Packerfile.
func (s *Stage) EXPOSECommand(options.EXPOSEOptions, ...options.EXPOSEOptions) error {
	panic("unimplemented")
}
