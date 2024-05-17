package executor

import (
	"context"

	"github.com/buildpacks/pack/internal/build"
	state "github.com/buildpacks/pack/internal/buildkit/build_state"
	"github.com/buildpacks/pack/internal/buildkit/builder"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

type LifecycleExecutor struct {
	logger  logging.Logger
	*builder.Builder[state.State]
	targets []dist.Target
	dockerClient build.DockerClient
}

func New(ctx context.Context, client build.DockerClient, s *state.State, logger logging.Logger, target []dist.Target) (l LifecycleExecutor, err error){
	bldr, err := builder.New[state.State](ctx, "", s)
	return LifecycleExecutor{
		dockerClient: client,
		logger:  logger,
		Builder: bldr,
		targets: target,
	}, err
}
