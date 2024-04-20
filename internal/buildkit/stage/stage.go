package stage

import "github.com/buildpacks/pack/internal/buildkit/packerfile"

var _ packerfile.Packerfile = (*Stage)(nil)

type Stage struct{}
