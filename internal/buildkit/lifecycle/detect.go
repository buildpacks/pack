package lifecycle

import (
	"context"
	"encoding/json"

	"github.com/moby/buildkit/client/llb"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/pack/internal/buildkit/cnb"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
	"github.com/buildpacks/pack/pkg/dist"
)

func (l *LifecycleExecution) Detect(ctx context.Context) error {
	errs, _ := errgroup.WithContext(ctx)
	for _, target := range l.targets {
		target := target
		errs.Go(func() error {
			return l.detect(ctx, target)
		})
	}

	return errs.Wait()
}

func (l *LifecycleExecution) detect(ctx context.Context, target dist.Target) (err error) {
	var (
		stateOps = make([]llb.StateOption, 0)
		runOps   = make([]llb.RunOption, 0)
	)

	flags := []string{"-app", mountpaths.MountPathsForOS(target.OS, l.opts.Workspace).AppDir()}
	if l.platformAPI.AtLeast("0.10") && l.hasExtensions() {
		stateOps = append(stateOps, llb.AddEnv(cnb.CNB_EXPERIMENTAL_MODE, cnb.WARN))
	}
	// TODO: Add CustomName for llb.State
	// llb.WithCustomName("Detect")
	l.state.AddArgs(l.withLogLevel()...)
	l.state.AddVolumes(l.opts.Volumes...)
	l.state.WithNetwork(l.opts.Network)
	// TODO: should we add [build.EnsureVolumeAccess]? currently buildkit doesn't fully support windows.
	// It doesn't mean [llb] too doesn't support!
	l.state.WithFlags(flags...)
	// Should we use CopyOutToMaybe like
	// ```go
	// 	CopyOutToMaybe(filepath.Join(l.mountPaths.layersDir(), "analyzed.toml"), l.tmpDir)))
	// ```
	// I think we can use same [l.state.State] across all phases and access those files!

	// commit state with latest v1.ConfigFile always before running Container
	cfgBytes, err := json.Marshal(l.state.ConfigFile)
	if err != nil {
		return err
	}

	tmpState, err := l.state.State.WithImageConfig(cfgBytes)
	l.state.State = &tmpState
	return err
}
