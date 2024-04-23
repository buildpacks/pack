package options

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options/types"

// NOTE: All the Options provided here might not work!

// NOTE: [CMDOptions] is embedded in [ENTRYPOINTOptions]!
// Remove the embed if any Options specific to [CMD] is added in future.

// You can specify CMD instructions using shell or exec forms
type CMD struct {
	Form types.Form
}
