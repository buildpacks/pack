package executor

import (
	"github.com/buildpacks/pack/internal/buildkit/state"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

type LifecycleExecutor struct {
	logger  logging.Logger
	state   state.State
	targets []dist.Target
}

func New(state state.State, logger logging.Logger, target []dist.Target) LifecycleExecutor {
	return LifecycleExecutor{
		logger:  logger,
		state:   state,
		targets: target,
	}
}
