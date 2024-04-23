package lifecycle

import (
	"context"

	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
)

func (l *LifecycleExecution) Build(ctx context.Context) error {
	mounter := mountpaths.MountPathsForOS(l.state.OS, l.opts.Workspace)
	flags := []string{"-app", mounter.AppDir()}
	l.state.AddArgs(l.withLogLevel()...).
		Network(l.opts.Network).
		AddVolumes(l.opts.Volumes...).
		WithFlags(flags...)
	// TODO: Add ll.WithInternalName("Builder")
}
