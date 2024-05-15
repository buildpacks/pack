package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	// gatewayClient "github.com/moby/buildkit/frontend/gateway/client"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/buildkit/builder"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
	"github.com/buildpacks/pack/internal/buildkit/state"

	// "github.com/buildpacks/pack/internal/buildkit/packerfile/options"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/pkg/cache"
)

func (l *LifecycleExecution) Create(ctx context.Context, c *client.Client, buildCache, launchCache build.Cache) error {
	mounter := mountpaths.MountPathsForOS(runtime.GOOS, l.opts.Workspace) // we are going to run a single container i.e the container with the current target's OS 
	var mounts = make([]gatewayClient.Mount, 0, 4)
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
		l.state = l.state.AddVolume(l.opts.Volumes...)
		for _, m := range l.opts.Volumes {
			mounts = append(mounts, gatewayClient.Mount{
				Dest: m,
				MountType: pb.MountType_CACHE,
				CacheOpt: &pb.CacheOpt{
					Sharing: pb.CacheSharingOpt_SHARED,
				},
			})
		}
	case cache.Volume, cache.Bind:
		volumes := append(l.opts.Volumes, fmt.Sprintf("%s:%s", buildCache.Name(), mounter.CacheDir()))
		l.state = l.state.AddVolume(volumes...)
		for _, m := range volumes {
			mounts = append(mounts, gatewayClient.Mount{
				Dest: m,
				MountType: pb.MountType_CACHE,
				CacheOpt: &pb.CacheOpt{
					Sharing: pb.CacheSharingOpt_SHARED,
				},
			})
		}
	}

	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		l.state = l.state.AddEnv(sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix())))
	}

	projectMetadata, err := json.Marshal(l.opts.ProjectMetadata)
	if err != nil {
		return err
	}
	flags = append(l.withLogLevel(flags...), l.opts.Image.String())
	// userPerm := fmt.Sprintf("%s:%s", strconv.Itoa(l.opts.Builder.UID()), strconv.Itoa(l.opts.Builder.GID()))
	l.state = l.state.Entrypoint("/cnb/lifecycle/creator").
		Network(l.opts.Network).
		Mkdir(mounter.CacheDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())). // create `/cache` dir
		Mkdir(l.opts.Workspace, fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())). // create `/workspace` dir
		Mkdir(
			mounter.LayersDir(), // create `/layers` dir for future reference
			fs.ModeDir, 
			llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()), // add uid and gid for for the given `layers` dir
		).
		MkFile(mounter.ProjectPath(), fs.ModePerm, projectMetadata, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())).
		Mkdir(mounter.AppDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())).
		// Use Add, cause: The AppPath can be either a directory or a tar file!
		// The [Add] command is responsible for extracting tar and fetching remote files!
		AddVolume(fmt.Sprintf("%s:%s", l.opts.AppPath, mounter.AppDir()))
	mounts = append(mounts, gatewayClient.Mount{
		Dest: fmt.Sprintf("%s:%s", l.opts.AppPath, mounter.AppDir()),
		MountType: pb.MountType_CACHE,
		CacheOpt: &pb.CacheOpt{Sharing: pb.CacheSharingOpt_SHARED},
	})
		// Add([]string{l.opts.AppPath}, mounter.AppDir(), options.ADD{Chown: userPerm, Chmod: userPerm, Link: true})
		// TODO: CopyOutTo(mounter.SbomDir(), l.opts.SBOMDestinationDir)
		// TODO: CopyOutTo(mounter.ReportPath(), l.opts.ReportDestinationDir)
		// TODO: CopyOut(l.opts.Termui.ReadLayers, mounter.LayersDir(), mounter.AppDir())))

	layoutDir := filepath.Join(paths.RootDir, "layout-repo")
	if l.opts.Layout {
		l.state = l.state.AddEnv("CNB_USE_LAYOUT", "true").
			AddEnv("CNB_LAYOUT_DIR", layoutDir).
			AddEnv("CNB_EXPERIMENTAL_MODE", "WARN").
			Mkdir(layoutDir, fs.ModeDir) // also create `layoutDir`
	}

	if l.opts.Publish || l.opts.Layout {
		authConfig, err := auth.BuildEnvVar(l.opts.Keychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		if err != nil {
			return err
		}
		_ = authConfig

		// can we use SecretAsEnv, cause The ENV is exposed in ConfigFile whereas SecretAsEnv not! But are we using DinD?
		// shall we add secret to builder instead of remodifying existing builder to add `CNB_REGISTRY_AUTH` 
		// we can also reference a file as secret!
		l.state = l.state.User(state.RootUser(runtime.GOOS)).AddArg(fmt.Sprintf("CNB_REGISTRY_AUTH=%s", "."))
	} else {
		// TODO: WithDaemonAccess(l.opts.DockerHost)

		flags = append(flags, "-daemon", "-launch-cache", mounter.LaunchCacheDir())
		l.state = l.state.AddVolume(fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()))
		mounts = append(mounts, gatewayClient.Mount{
			Dest: fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()),
			MountType: pb.MountType_CACHE,
			CacheOpt: &pb.CacheOpt{Sharing: pb.CacheSharingOpt_SHARED},
		})
	}
	// TODO: uncomment below line
	l.state = l.state.Cmd(flags...) // .Run([]string{"/cnb/lifecycle/creator", "-app", "/workspace", "-cache-dir", "/cache", "-run-image", "ghcr.io/jericop/run-jammy:latest", "wygin/react-yarn"}, func(state llb.ExecState) llb.State {return state.State})
	// TODO: delete below line
	// l.state = l.state.AddArg(flags...)

	// var platforms = make([]v1.Platform, 0)
	// for _, target := range l.targets {
	// 	target.Range(ctx, func(t dist.Target) error {
	// 		platforms = append(platforms, v1.Platform{
	// 			OS:           t.OS,
	// 			Architecture: t.Arch,
	// 			Variant:      t.ArchVariant,
	// 			// TODO: add more fields
	// 		})
	// 		return nil
	// 	})
	// }

	bldr := builder.New(l.opts.Image.Name(), l.state, mounts)
	bldr.Entrypoint("/cnb/lifecycle/creator")
	bldr.Cmd(flags)
	bldr.User(state.RootUser(runtime.GOOS))
	// bldr.AddEnv(fmt.Sprintf("%s=%s", sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix()))))
	bldr.AddEnv(fmt.Sprintf("%s=%s", "CNB_USE_LAYOUT", "true"))
	bldr.AddEnv(fmt.Sprintf("%s=%s", "CNB_LAYOUT_DIR", layoutDir))
	bldr.AddEnv(fmt.Sprintf("%s=%s", "CNB_EXPERIMENTAL_MODE", "WARN"))
	bldr.AddPlatform(v1.Platform{
		OS: "linux",
		Architecture: "amd64",
	})
	bldr.Stdout()
	bldr.Stderr()
	// res, err := bldr.Build(appcontext.Context(), *c)
	// var status = make(chan *client.SolveStatus)
	resp, err := c.Build(ctx, client.SolveOpt{
		Exports: []client.ExportEntry{
			{
				Type: client.ExporterOCI,
				Output: func(m map[string]string) (io.WriteCloser, error) {
					return os.Create(filepath.Join("exports", "react-yarn-builder.tar"))
				},
				Attrs: map[string]string{
					"name": "localhost:3000/wygin/react-yarn-builder",
					"tar": "true",
					"push": "true",
				},
			},
			{
				Type: "local",
				OutputDir: filepath.Join("exports", "local"),
			},
		},
		CacheExports: []client.CacheOptionsEntry{
			{
				Type: "local",
				Attrs: map[string]string{
					"dest": filepath.Join("DinD", "cache"),
				},
			},
		},
		CacheImports: []client.CacheOptionsEntry{
			{
				Type: "local",
				Attrs: map[string]string{
					"src": filepath.Join("DinD", "cache"),
				},
			},
		},
	}, "packerfile.v0", bldr.Build, nil)
	if resp != nil {
		fmt.Printf("configFile current: \n%s\n", resp.ExporterResponse[exptypes.ExporterImageConfigKey])
	}

	if err != nil {
		return err
	}

	// ctx2, cancel := context.WithCancel(context.TODO())
	// defer cancel()
	// printer, err := progress.NewPrinter(ctx, os.Stderr, "plain")
	// if err != nil {
	// 	return err
	// }

	// select {
	// case status := <- status:
	// 	fmt.Printf("status: \n")
	// 	printer.Write(status)
	// default:
	// 	return err
	// }
	// if err := grpcclient.RunFromEnvironment(appcontext.Context(),bldr.Build); err != nil {
	// 	return err
	// }
	// if err := grpcclient.RunFromEnvironment(ctx, bldr.Build); err != nil {
	// 	l.logger.Errorf("fatal error: %+v", err)
	// }
	
	// I don't think we need to export DinD image!
	// Instead of using volumes for caching let's use CacheImports and CacheExports.
	//
	// var sloveStatus chan *client.SolveStatus = make(chan *client.SolveStatus)

	return err
}
