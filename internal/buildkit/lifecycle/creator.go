package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"

	// gatewayClient "github.com/moby/buildkit/frontend/gateway/client"

	"github.com/buildpacks/pack/internal/build"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"

	// "github.com/buildpacks/pack/internal/buildkit/packerfile/options"
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
		// l.state = l.state.AddVolume(l.opts.Volumes...)
		for _, m := range l.opts.Volumes {
			host, _, _ := strings.Cut(m, ":") // l.opts.Volumes in format /host:/conatiner:RO
			mounts = append(mounts, gatewayClient.Mount{
				Dest: host,
				MountType: pb.MountType_CACHE,
				CacheOpt: &pb.CacheOpt{
					Sharing: pb.CacheSharingOpt_SHARED,
				},
			})
		}
	case cache.Volume, cache.Bind:
		volumes := append(l.opts.Volumes, fmt.Sprintf("%s:%s", buildCache.Name(), mounter.CacheDir()))
		// l.state = l.state.AddVolume(volumes...)
		for _, m := range volumes {
			v, _, _ := strings.Cut(m, ":") // l.opts.Volumes in format /host:/conatiner:RO
			mounts = append(mounts, gatewayClient.Mount{
				Dest: v,
				MountType: pb.MountType_BIND,
			})
		}
	}

	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		// l.state = l.state.AddEnv(sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix())))
	}

	// projectMetadata, err := json.Marshal(l.opts.ProjectMetadata)
	// if err != nil {
	// 	return err
	// }
	flags = append(l.withLogLevel(flags...), l.opts.Image.String())
	// userPerm := fmt.Sprintf("%s:%s", strconv.Itoa(l.opts.Builder.UID()), strconv.Itoa(l.opts.Builder.GID()))
	// l.state = l.state.Entrypoint("/cnb/lifecycle/creator").
	// 	Network(l.opts.Network).
	// 	Mkdir(mounter.CacheDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())). // create `/cache` dir
	// 	Mkdir(l.opts.Workspace, fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())). // create `/workspace` dir
	// 	Mkdir(
	// 		mounter.LayersDir(), // create `/layers` dir for future reference
	// 		fs.ModeDir, 
	// 		llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()), // add uid and gid for for the given `layers` dir
	// 	).
	// 	MkFile(mounter.ProjectPath(), fs.ModePerm, projectMetadata, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())).
	// 	Mkdir(mounter.AppDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())).
	// 	// Use Add, cause: The AppPath can be either a directory or a tar file!
	// 	// The [Add] command is responsible for extracting tar and fetching remote files!
	// 	AddVolume(fmt.Sprintf("%s:%s", l.opts.AppPath, mounter.AppDir()))
	mounts = append(mounts, gatewayClient.Mount{
		Dest: l.opts.AppPath, // fmt.Sprintf("%s:%s", l.opts.AppPath, mounter.AppDir()),
		MountType: pb.MountType_CACHE,
		CacheOpt: &pb.CacheOpt{Sharing: pb.CacheSharingOpt_SHARED},
	})
		// Add([]string{l.opts.AppPath}, mounter.AppDir(), options.ADD{Chown: userPerm, Chmod: userPerm, Link: true})
		// TODO: CopyOutTo(mounter.SbomDir(), l.opts.SBOMDestinationDir)
		// TODO: CopyOutTo(mounter.ReportPath(), l.opts.ReportDestinationDir)
		// TODO: CopyOut(l.opts.Termui.ReadLayers, mounter.LayersDir(), mounter.AppDir())))

	// layoutDir := filepath.Join(paths.RootDir, "layout-repo")
	if l.opts.Layout {
		// l.state = l.state.AddEnv("CNB_USE_LAYOUT", "true").
		// 	AddEnv("CNB_LAYOUT_DIR", layoutDir).
		// 	AddEnv("CNB_EXPERIMENTAL_MODE", "WARN").
		// 	Mkdir(layoutDir, fs.ModeDir) // also create `layoutDir`
	}

	if l.opts.Publish || l.opts.Layout {
		// authConfig, err := auth.BuildEnvVar(l.opts.Keychain, l.opts.Image.String(), l.opts.RunImage, l.opts.CacheImage, l.opts.PreviousImage)
		// if err != nil {
		// 	return err
		// }

		// can we use SecretAsEnv, cause The ENV is exposed in ConfigFile whereas SecretAsEnv not! But are we using DinD?
		// shall we add secret to builder instead of remodifying existing builder to add `CNB_REGISTRY_AUTH` 
		// we can also reference a file as secret!
		// l.state = l.state.User(state.RootUser(runtime.GOOS)).AddArg(fmt.Sprintf("CNB_REGISTRY_AUTH=%s", authConfig))
	} else {
		// TODO: WithDaemonAccess(l.opts.DockerHost)

		flags = append(flags, "-daemon", "-launch-cache", mounter.LaunchCacheDir())
		// l.state = l.state.AddVolume(fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()))
		mounts = append(mounts, gatewayClient.Mount{
			Dest: launchCache.Name(), // fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()),
			MountType: pb.MountType_CACHE,
			CacheOpt: &pb.CacheOpt{Sharing: pb.CacheSharingOpt_SHARED},
		})
	}
	// TODO: uncomment below line
	// l.state = l.state.Cmd(flags...) // .Run([]string{"/cnb/lifecycle/creator", "-app", "/workspace", "-cache-dir", "/cache", "-run-image", "ghcr.io/jericop/run-jammy:latest", "wygin/react-yarn"}, func(state llb.ExecState) llb.State {return state.State})
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

	// l.state.Run(append([]string{"/cnb/lifecycle/creator"}, flags...), func(state llb.ExecState) llb.State {return state.Root()})
	// bldr := builder.New(l.opts.Image.Name(), l.state, mounts)
	// bldr.Entrypoint("/cnb/lifecycle/creator")
	// bldr.Cmd(flags)
	// bldr.User(state.RootUser(runtime.GOOS))
	// // bldr.AddEnv(fmt.Sprintf("%s=%s", sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix()))))
	// bldr.AddEnv(fmt.Sprintf("%s=%s", "CNB_USE_LAYOUT", "true"))
	// bldr.AddEnv(fmt.Sprintf("%s=%s", "CNB_LAYOUT_DIR", layoutDir))
	// bldr.AddEnv(fmt.Sprintf("%s=%s", "CNB_EXPERIMENTAL_MODE", "WARN"))
	// bldr.AddPlatform(v1.Platform{
	// 	OS: "linux",
	// 	Architecture: "amd64",
	// })
	// bldr.Stdout()
	// bldr.Stderr()
	// var status = make(chan *client.SolveStatus)
	// baseStatus := status

	// printer, err := progress.NewPrinter(ctx, os.Stderr, "plain")
	// if err != nil {
	// 	return err
	// }
	// go func() {
	// 	for {
	// 		s, ok := <-status
	// 		if !ok {
	// 			return
	// 		}
	// 		printer.Write(s)
	// 		select {
	// 		case <-baseStatus:
	// 			// base channel is closed, discard status messages
	// 		default:
	// 			baseStatus <- s
	// 		}
	// 	}
	// }()
	// defer close(baseStatus)

	// _, err := c.Build(ctx, client.SolveOpt{
	// 	AllowedEntitlements: []entitlements.Entitlement{
	// 		entitlements.EntitlementNetworkHost,
	// 	},
	// 	// Exports: []client.ExportEntry{
	// 	// 	{
	// 	// 		Type: client.ExporterOCI,
	// 	// 		Output: func(m map[string]string) (io.WriteCloser, error) {
	// 	// 			return os.Create(filepath.Join("exports", "react-yarn-builder.tar"))
	// 	// 		},
	// 	// 		Attrs: map[string]string{
	// 	// 			"name": "localhost:3000/wygin/react-yarn-builder",
	// 	// 			"tar": "true",
	// 	// 			"push": "true",
	// 	// 		},
	// 	// 	},
	// 	// },
	// 	// CacheExports: []client.CacheOptionsEntry{
	// 	// 	{
	// 	// 		Type: "local",
	// 	// 		Attrs: map[string]string{
	// 	// 			"dest": filepath.Join("DinD", "cache"),
	// 	// 		},
	// 	// 	},
	// 	// },
	// 	// CacheImports: []client.CacheOptionsEntry{
	// 	// 	{
	// 	// 		Type: "local",
	// 	// 		Attrs: map[string]string{
	// 	// 			"src": filepath.Join("DinD", "cache"),
	// 	// 		},
	// 	// 	},
	// 	// },
	// }, "", bldr.Build, status)
	// if resp != nil {
	// 	fmt.Printf("configFile current: \n%s\n", resp.ExporterResponse[exptypes.ExporterImageConfigKey])
	// }



	_, err := c.Build(ctx, client.SolveOpt{
		AllowedEntitlements: []entitlements.Entitlement{
			entitlements.EntitlementNetworkHost,
		},
	}, "", func(ctx context.Context, c gatewayClient.Client) (res *gatewayClient.Result, err error) {
		st := llb.Image("busybox:latest", llb.WithMetaResolver(c), llb.LinuxAmd64)
		def, err := st.Marshal(ctx)
		if err != nil {
			return nil, err
		}

		res, err = c.Solve(ctx, gatewayClient.SolveRequest{
			Evaluate: true,
			Definition: def.ToPB(),
		})

		ctr, err := c.NewContainer(ctx, gatewayClient.NewContainerRequest{
			Platform: &pb.Platform{
				OS: "linux",
				Architecture: "amd64",
			},
		})
		if err != nil {
			return nil, err
		}
		defer ctr.Release(ctx)

		pid, err := ctr.Start(ctx, gatewayClient.StartRequest{
			// Args: []string{"sleep", "10", "&&", "echo", "hello world!"},
		})
		if err != nil {
			return res, err
		}

		return res, pid.Wait()
	}, nil)
	return err
}
