package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pelletier/go-toml"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
	"github.com/buildpacks/pack/pkg/cache"
)

func (l *LifecycleExecution) Analyzer(ctx context.Context, buildCache, launchCache build.Cache) error {
	// errs, _ := errgroup.WithContext(ctx)
	// for _, target := range l.targets {
	// target := target
	// errs.Go(func() error {
	return l.analyzer(ctx, buildCache, launchCache)
	// })
	// }

	// return errs.Wait()
}

func (l *LifecycleExecution) analyzer(ctx context.Context, buildCache, launchCache build.Cache) (err error) {
	var flags []string
	args := []string{l.opts.Image.String()}
	platformAPILessThan07 := l.platformAPI.LessThan("0.7")

	// TODO: llb.WithInternerName("Analyzer")
	mounter := mountpaths.MountPathsForOS(l.state.OS, l.opts.Workspace)
	if l.opts.ClearCache {
		if platformAPILessThan07 || l.platformAPI.AtLeast("0.9") {
			args = prependArg("-skip-layers", args)
		}
	} else {
		switch buildCache.Type() {
		case cache.Image:
			flags = append(flags, "-cache-image", buildCache.Name())
		case cache.Volume:
			if platformAPILessThan07 {
				args = append([]string{"-cache-dir", mounter.CacheDir()}, args...)
				l.state.AddVolumes(fmt.Sprintf("%s:%s", buildCache.Name(), mounter.CacheDir()))
			}
		}
	}

	if l.platformAPI.AtLeast("0.9") {
		if !l.opts.Publish {
			args = append([]string{"-launch-cache", mounter.LaunchCacheDir()}, args...)
			l.state.AddVolumes(fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()))
		}
	}

	if l.opts.GID >= overrideGID {
		flags = append(flags, "-gid", strconv.Itoa(l.opts.GID))
	}

	if l.opts.UID >= overrideUID {
		flags = append(flags, "-uid", strconv.Itoa(l.opts.UID))
	}

	if l.opts.PreviousImage != "" {
		if l.opts.Image == nil {
			return errors.New("image can't be nil")
		}

		image, err := name.ParseReference(l.opts.Image.Name(), name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid image name: %s", err)
		}

		prevImage, err := name.ParseReference(l.opts.PreviousImage, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("invalid previous image name: %s", err)
		}
		if l.opts.Publish {
			if image.Context().RegistryStr() != prevImage.Context().RegistryStr() {
				return fmt.Errorf(`when --publish is used, <previous-image> must be in the same image registry as <image>
	            image registry = %s
	            previous-image registry = %s`, image.Context().RegistryStr(), prevImage.Context().RegistryStr())
			}
		}
		if platformAPILessThan07 {
			l.opts.Image = prevImage
		} else {
			args = append([]string{"-previous-image", l.opts.PreviousImage}, args...)
		}
	}

	if !platformAPILessThan07 {
		for _, tag := range l.opts.AdditionalTags {
			args = append([]string{"-tag", tag}, args...)
		}
		if l.opts.RunImage != "" {
			args = append([]string{"-run-image", l.opts.RunImage}, args...)
		}
		if l.platformAPI.LessThan("0.12") {
			args = append([]string{"-stack", mounter.StackPath()}, args...)
			buf := &bytes.Buffer{}
			stackMetadata := l.opts.Builder.Stack()
			if err := toml.NewEncoder(buf).Encode(&stackMetadata); err != nil {
				return err
			}
			l.state.MkFile(mounter.StackPath(), os.ModePerm, buf)
		} else {
			args = append([]string{"-run", mounter.RunPath()}, args...)
			buf := &bytes.Buffer{}
			runMetadata := l.opts.Builder.RunImages()
			if err := toml.NewEncoder(buf).Encode(&runMetadata); err != nil {
				return err
			}
			l.state.MkFile(mounter.RunPath(), os.ModePerm, buf)
		}
	}

	withLayoutOperation(&l.state)
	l.state.AddFlags(flags...)
	l.state.WithImage(l.opts.LifecycleImage)
	l.state.AddEnvf(builder.EnvUID, "%d", l.opts.Builder.UID())
	l.state.AddEnvf(builder.EnvGID, "%d", l.opts.Builder.GID())
	l.state.Network(l.opts.Network)

	if l.opts.Publish || l.opts.Layout {
		authConfig, err := auth.BuildEnvVar(l.opts.Keychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}
		l.state = l.state.AddEnv("CNB_REGISTRY_AUTH", authConfig)
		l.state.Root()
		l.state.AddArgs(l.withLogLevel(args...)...)
	} else {
		l.state.AddArgs(l.withLogLevel("-daemon")...)
		// TODO: Add DockerHost
		// WithDaemonAccess(l.opts.DockerHost)
	}

	return err
}
