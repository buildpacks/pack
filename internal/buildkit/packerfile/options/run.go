package options

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options/types"

// NOTE: All the Options provided here might not work!

type RUNOptions struct {
	Mount    types.Mount
	Network  types.Network
	Security types.Security
}
