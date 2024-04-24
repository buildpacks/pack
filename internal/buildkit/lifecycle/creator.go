package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/buildkit/cnb"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/pkg/cache"
)

func (l *LifecycleExecution) Create(ctx context.Context, buildCache, launchCache build.Cache) error {
	mounter := mountpaths.MountPathsForOS(l.state.OS, l.opts.Workspace)
	flags := addTags([]string{
		"-app", mounter.AppDir(),
		"-cache-dir", mounter.CacheDir(),
		"-run-image", l.opts.RunImage,
	}, l.opts.AdditionalTags)

	if l.opts.ClearCache {
		flags = append(flags, "-skip-restore")
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

		flags = append(flags, "-previous-image", l.opts.PreviousImage)
	}

	processType := determineDefaultProcessType(l.platformAPI, l.opts.DefaultProcessType)
	if processType != "" {
		flags = append(flags, "-process-type", processType)
	}

	switch buildCache.Type() {
	case cache.Image:
		flags = append(flags, "-cache-image", buildCache.Name())
		l.state = l.state.AddVolumes(l.opts.Volumes...)
	case cache.Volume, cache.Bind:
		l.state = l.state.AddVolumes(append(l.opts.Volumes, fmt.Sprintf("%s:%s", buildCache.Name(), mounter.CacheDir()))...)
	}

	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		l.state = l.state.AddEnv(sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix())))
	}

	l.state.WithFlags(l.withLogLevel(flags...)...).
		WithArgs(l.opts.Image.String()).
		Network(l.opts.Network).
		MkFile(mounter.ProjectPath(), l.opts.ProjectMetadata).
		Add(l.opts.AppPath, mounter.AppDir(), options.ADD{Chown: fmt.Sprintf("%s:%s", l.opts.Builder.UID(), l.opts.Builder.GID())}).
		// Lets mount the Volume instead of copying data
		// TODO: make this volume readonly
		AddVolumes(fmt.Sprintf("%s:%s", l.opts.AppPath, mounter.AppDir()))
		// TODO: CopyOutTo(mounter.SbomDir(), l.opts.SBOMDestinationDir)
		// TODO: CopyOutTo(mounter.ReportPath(), l.opts.ReportDestinationDir)
		// TODO: CopyOut(l.opts.Termui.ReadLayers, mounter.LayersDir(), mounter.AppDir())))

	if l.opts.Layout {
		layoutDir := filepath.Join(paths.RootDir, "layout-repo")
		l.state = l.state.AddEnv("CNB_USE_LAYOUT", "true").
			AddEnv("CNB_LAYOUT_DIR", layoutDir).
			AddEnv(cnb.CnbExperimentalMode, cnb.WARN)
	}

	if l.opts.Publish || l.opts.Layout {
		authConfig, err := auth.BuildEnvVar(l.opts.Keychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}

		l.state = l.state.Root().AddArgs(fmt.Sprintf("CNB_REGISTRY_AUTH=%s", authConfig))
	} else {
		// TODO: WithDaemonAccess(l.opts.DockerHost)
		// Maybe adding ExtraHost?

		l.state = l.state.WithFlags("-daemon", "-launch-cache", mounter.LaunchCacheDir()).
			AddVolumes(fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()))
	}

	// TODO: Till this point the llb.State is creating a DAG Graph.
	// We need to implement Solver to solve this DAG Graph in the most efficient way and then
	// use the [client/Result] or [client.Response] to create a container out of it
	// we might not need to export the Result to the demon as we are deleting conatiner after building Image, in [build] package
	// So we directly start the new container without exporting image and return the error
	// if the container fails we return the error.
	return nil
}
