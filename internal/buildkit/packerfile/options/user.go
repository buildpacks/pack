package options

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options/types"

type USER struct {
	types.UID // REQUIRED.
	types.GID // OPTIONAL.
}
