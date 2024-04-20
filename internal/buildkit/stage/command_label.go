package stage

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options"

// LABELCommand implements packerfile.Packerfile.
func (s *Stage) LABELCommand(options.LABELOptions, ...options.LABELOptions) error {
	panic("unimplemented")
}
