package lifecycle

import (
	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/buildkit/state"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

func NewLifecycleExecution(logger logging.Logger, state state.State, targets []dist.Target, tmpDir string, opts build.LifecycleOptions) (*LifecycleExecution, error) {
	supportedPlatformAPIs := append(
		opts.Builder.LifecycleDescriptor().APIs.Platform.Deprecated,
		opts.Builder.LifecycleDescriptor().APIs.Platform.Supported...,
	)

	latestSupportedPlatformAPI, err := build.FindLatestSupported(supportedPlatformAPIs, opts.LifecycleApis)
	exec := &LifecycleExecution{
		logger:       logger,
		state:        state,
		layersVolume: paths.FilterReservedNames("pack-layers-" + randString(10)),
		appVolume:    paths.FilterReservedNames("pack-app-" + randString(10)),
		platformAPI:  latestSupportedPlatformAPI,
		opts:         opts,
		tmpDir:       tmpDir,
		targets:      targets,
	}

	if opts.Interactive {
		exec.logger = opts.Termui
	}

	return exec, err
}
