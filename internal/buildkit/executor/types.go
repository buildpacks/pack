package executor

import (
	"github.com/buildpacks/pack/internal/build"
	state "github.com/buildpacks/pack/internal/buildkit/build_state"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

type LifecycleExecutor struct {
	logger  logging.Logger
	state   state.State
	targets []dist.Target
	dockerClient build.DockerClient
}

func New(client build.DockerClient, state state.State, logger logging.Logger, target []dist.Target) LifecycleExecutor {
	return LifecycleExecutor{
		dockerClient: client,
		logger:  logger,
		state:   state,
		targets: target,
	}
}
