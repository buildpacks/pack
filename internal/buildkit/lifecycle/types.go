package lifecycle

import (
	"github.com/buildpacks/lifecycle/api"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/buildkit/state"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

type LifecycleExecution struct {
	logger       logging.Logger
	state        state.State
	platformAPI  *api.Version
	layersVolume string
	appVolume    string
	targets      []dist.Target
	opts         build.LifecycleOptions
	tmpDir       string
}
