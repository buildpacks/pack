package options

import "github.com/buildpacks/pack/internal/buildkit/packerfile/options/types"

// NOTE: All the Options provided here might not work!

// The EXPOSE instruction informs Docker that the container listens on the
// specified network ports at runtime. supported [TCP or UDP]. Defaults to [TCP]
type EXPOSE struct {
	Port     string         // REQUIRED.
	Protocol types.Protocol // OPTIONAL.
}
