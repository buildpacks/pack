package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/pack/internal/build"
	state "github.com/buildpacks/pack/internal/buildkit/build_state"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/pkg/cache"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
)

func (l *LifecycleExecution) Create(ctx context.Context, c *client.Client, buildCache, launchCache build.Cache) error {
	// TODO: move mounter into [l.[*builder.Builder[state.State]]]
	mounter := mountpaths.MountPathsForOS(runtime.GOOS, l.opts.Workspace) // we are going to run a single container i.e the container with the current target's OS 
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
		l.AddVolume(l.opts.Volumes...)
	case cache.Volume, cache.Bind:
		volumes := append(l.opts.Volumes, fmt.Sprintf("%s:%s", buildCache.Name(), mounter.CacheDir()))
		l.AddVolume(volumes...)
	}

	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		l.AddEnv(sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix()))) // I think this env is set on builder
	}

	projectMetadata, err := json.Marshal(l.opts.ProjectMetadata)
	if err != nil {
		return err
	}
	flags = append(l.withLogLevel(flags...), l.opts.Image.String())
	// userPerm := fmt.Sprintf("%s:%s", strconv.Itoa(l.opts.Builder.UID()), strconv.Itoa(l.opts.Builder.GID()))
	l.Entrypoint("/cnb/lifecycle/creator").
		Network(l.opts.Network).
		Mkdir(mounter.CacheDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())). // create `/cache` dir
		Mkdir(l.opts.Workspace, fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())). // create `/workspace` dir
		Mkdir(
			mounter.LayersDir(), // create `/layers` dir for future reference
			fs.ModeDir, 
			llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()), // add uid and gid for for the given `layers` dir
		).
		MkFile(mounter.ProjectPath(), fs.ModePerm, projectMetadata, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())).
		Mkdir(mounter.AppDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()))
		// Use Add, cause: The AppPath can be either a directory or a tar file!
		// The [Add] command is responsible for extracting tar
		// TODO: CopyOutTo(mounter.SbomDir(), l.opts.SBOMDestinationDir)
		// TODO: CopyOutTo(mounter.ReportPath(), l.opts.ReportDestinationDir)
		// TODO: CopyOut(l.opts.Termui.ReadLayers, mounter.LayersDir(), mounter.AppDir())))

	l.UID(fmt.Sprintf("%d", l.opts.Builder.UID())).
		GID(fmt.Sprintf("%d", l.opts.Builder.GID())).
		AppSource(l.opts.AppPath, mounter.AppDir())

	layoutDir := filepath.Join(paths.RootDir, "layout-repo")
	if l.opts.Layout {
		l.AddEnv("CNB_USE_LAYOUT", "true").
			AddEnv("CNB_LAYOUT_DIR", layoutDir).
			AddEnv("CNB_EXPERIMENTAL_MODE", "WARN")
			// Mkdir(layoutDir, fs.ModeDir) // also create `layoutDir`
	}

	if l.opts.Publish || l.opts.Layout {
		authConfig, err := auth.BuildEnvVar(l.opts.Keychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}

		fmt.Printf("using auth config for push image: %s\n", authConfig)
		// can we use SecretAsEnv, cause The ENV is exposed in ConfigFile whereas SecretAsEnv not! But are we using DinD?
		// shall we add secret to builder instead of remodifying existing builder to add `CNB_REGISTRY_AUTH` 
		// we can also reference a file as secret!
		l.User(state.RootUser(runtime.GOOS)).
			AddEnv(fmt.Sprintf("CNB_REGISTRY_AUTH=%s", authConfig))
	} else {
		// TODO: WithDaemonAccess(l.opts.DockerHost)

		flags = append(flags, "-daemon", "-launch-cache", mounter.LaunchCacheDir())
		l.AddVolume(fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()))
	}
	l.Cmd(flags...) // .Run([]string{"/cnb/lifecycle/creator", "-app", "/workspace", "-cache-dir", "/cache", "-run-image", "ghcr.io/jericop/run-jammy:latest", "wygin/react-yarn"}, func(state llb.ExecState) llb.State {return state.State})
	// TODO: delete below line
	// l.state = l.state.AddArg(flags...)

	var platforms = make([]v1.Platform, 0)
	for _, target := range l.targets {
		target.Range(func(t dist.Target) error {
			platforms = append(platforms, v1.Platform{
				OS:           t.OS,
				Architecture: t.Arch,
				Variant:      t.ArchVariant,
				// TODO: add more fields
			})
			return nil
		})
	}

	return l.Build(ctx)
}
