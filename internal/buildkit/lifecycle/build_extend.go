package lifecycle

// import (
// 	"context"
// 	"fmt"

// 	"github.com/buildpacks/pack/internal/build"
// 	"github.com/buildpacks/pack/internal/buildkit/cnb"
// 	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
// )

// func (l *LifecycleExecution) ExtendBuild(ctx context.Context, kanikoCache build.Cache) error {
// 	mounter := mountpaths.MountPathsForOS(l.state.OS, l.opts.Workspace)
// 	flags := []string{"-app", mounter.AppDir()}
// 	// TODO: llb.WithCustomName("extender (build)")
// 	l.state.AddArgs(l.withLogLevel()...).
// 		AddVolumes(l.opts.Volumes...).
// 		AddEnv(cnb.CnbExperimentalMode, cnb.WARN).
// 		AddVolumes(fmt.Sprintf("%s:%s", kanikoCache.Name(), mounter.KanikoCacheDir())).
// 		Network(l.opts.Network).
// 		WithFlags(flags...).
// 		Root()
// }
