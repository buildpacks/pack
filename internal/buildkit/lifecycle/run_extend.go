package lifecycle

// import (
// 	"context"

// 	"github.com/buildpacks/pack/internal/build"
// 	"github.com/buildpacks/pack/internal/buildkit/cnb"
// 	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
// )

// func (l *LifecycleExecution) ExtendRun(ctx context.Context, kanikoCache build.Cache, runImageName string) error {
// 	mounter := mountpaths.MountPathsForOS(l.state.OS, l.opts.Workspace)
// 	flags := []string{"-app", mounter.AppDir(), "-kind", "run"}
// 	// TODO: llb.WithInternalName("extender (run)")

// 	l.state.AddArgs(l.withLogLevel()...).
// 		AddVolumes(l.opts.Volumes...).
// 		AddVolumes(fmt.Sprintf("%s:%s", kanikoCache.Name(), mounter.kanikoCacheDir())),
// 		AddEnv(cnb.CnbExperimentalMode, cnb.WARN).
// 		WithFlags(flags...).
// 		Network(l.opts.Network).
// 		Root().
// 		WithImage(runImageName)

// 	return nil
// }
