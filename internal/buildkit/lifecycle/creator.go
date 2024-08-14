package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	// "io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

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
	gwClient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	"github.com/moby/buildkit/util/progress/progresswriter"
	"github.com/tonistiigi/fsutil"
)

func (l *LifecycleExecution) Create(ctx context.Context, c *client.Client, buildCache, launchCache build.Cache) error {
	// TODO: move mounter into [l.[*builder.Builder[state.State]]]
	mounter := mountpaths.MountPathsForOS(runtime.GOOS, l.opts.Workspace) // we are going to run a single container i.e the container with the current target's OS
	fmt.Printf("using %q as workspace\n", mounter.AppDir())
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
		Mkdir(l.opts.Workspace, fs.ModePerm, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID())).   // create `/workspace` dir
		Mkdir(
			mounter.LayersDir(), // create `/layers` dir for future reference
			fs.ModeDir,
			llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()), // add uid and gid for for the given `layers` dir
		).
		MkFile(mounter.ProjectPath(), fs.ModePerm, projectMetadata, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()))
		// Mkdir(mounter.AppDir(), fs.ModeDir, llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()))
	// Use Add, cause: The AppPath can be either a directory or a tar file!
	// The [Add] command is responsible for extracting tar
	// TODO: CopyOutTo(mounter.SbomDir(), l.opts.SBOMDestinationDir)
	// TODO: CopyOutTo(mounter.ReportPath(), l.opts.ReportDestinationDir)
	// TODO: CopyOut(l.opts.Termui.ReadLayers, mounter.LayersDir(), mounter.AppDir())))

	fmt.Printf("copying files by chown: %d:%d\n", l.opts.Builder.UID(), l.opts.Builder.GID())
	pw, err := progresswriter.NewPrinter(context.TODO(), os.Stderr, "plain")
	if err != nil {
		return err
	}
	mw := progresswriter.NewMultiWriter(pw)

	// creating a llb.State by copying app src code by creating a new stage
	appSrc := llb.Local(l.opts.AppPath, llb.WithCustomNamef("Mounting Volume: %s", l.opts.AppPath))
	// var mode *os.FileMode
	// p, err := strconv.ParseUint(fmt.Sprintf("777"/*"%d:%d", l.opts.Builder.UID(), l.opts.Builder.GID()), 777, 32*/)
	// if err == nil {
	// 	perm := os.FileMode(p)
	// 	mode = &perm
	// }

	// FROM scratch as APPSRC
	// COPY / /

	// FROM paketobuildpacks/builder-jammy-tiny:0.0.250
	// COPY --from=APPSRC . /workspace

	var fmode = fs.ModePerm
	appScrCopy := llb.Copy(
		appSrc,
		"/", // copy root of appSrc
		mounter.AppDir(), // `/workspace`
		&llb.CopyInfo{
			Mode: &fmode,
			// IncludePatterns: []string{"/*"},
			FollowSymlinks: true,
			AttemptUnpack: true,
			CreateDestPath: true,
			AllowWildcard: true,
			AllowEmptyWildcard: true,
			CopyDirContentsOnly: true,
			ChownOpt: &llb.ChownOpt{
				// User: &llb.UserOpt{},
				// Group: &llb.UserOpt{Name: "cnb"},
				User: &llb.UserOpt{UID: 1000},
				Group: &llb.UserOpt{UID: 1001},
			},
		},
		llb.WithUIDGID(l.opts.Builder.UID(), l.opts.Builder.GID()),
	)
	llbState := llb.Image("paketobuildpacks/builder-jammy-tiny:0.0.250", llb.WithCustomName("pulling builder"))
	llbState = llbState.File(
		appScrCopy,
		llb.WithCustomNamef("COPY %s %s", l.opts.AppPath, mounter.AppDir()),
	)
	// llbState = llbState.File(
	// 	appScrCopy,
	// 	llb.WithCustomNamef("COPY %s %s", l.opts.AppPath, mounter.AppDir()),
	// )

	def, err := llbState.Marshal(ctx)
	if err != nil {
		return err
	}

	workspaceLocalMount, err := fsutil.NewFS(l.opts.AppPath)
	if err != nil {
		return err
	}

	var statFile func(ref gwClient.Reference, path string) error
	statFile = func(ref gwClient.Reference, path string) error {
		stat, err := ref.ReadDir(ctx, gwClient.ReadDirRequest{
			Path: path,
		})
		if err != nil {
			return err
		}

		if len(stat) == 0 {
			l.logger.Errorf("no files found in %s directory", path)
		}

		for _, f := range stat {
			if f.IsDir() {
				if err := statFile(ref, filepath.Join(path, f.GetPath())); err != nil {
					return err
				}
			}
			l.logger.Warnf("found %+v \n", f.GetPath(), f.GetSize_(), f)
		}
		return nil
	}

	fmt.Printf("exporting docker image with name: %s\n\n", l.opts.Image.Name())
	_, err = c.Build(ctx, client.SolveOpt{
		// Exports: []client.ExportEntry{
		// 	{
		// 		Type: client.ExporterDocker,
		// 		Output: func(m map[string]string) (io.WriteCloser, error) {
		// 			return os.Create(filepath.Join(l.opts.AppPath, "exports", fmt.Sprintf("%s.tar", l.opts.Image.Identifier())))
		// 		},
		// 	},
		// },
		LocalMounts: map[string]fsutil.FS{
			l.opts.AppPath: workspaceLocalMount,
		},
		AllowedEntitlements: []entitlements.Entitlement{entitlements.EntitlementNetworkHost},
	}, "", func(ctx context.Context, c gwClient.Client) (*gwClient.Result, error) {
		res, err := c.Solve(ctx, gwClient.SolveRequest{
			Evaluate:   true,
			Definition: def.ToPB(),
		})
		if err != nil {
			return res, err
		}

		ref, err := res.SingleRef()
		if err != nil {
			return res, err
		}

		if err = statFile(ref, mounter.AppDir()); err != nil {
			return res, err
		}

		ctr, err := c.NewContainer(ctx, gwClient.NewContainerRequest{
			Mounts: []gwClient.Mount{
				{
					Dest:      "/",
					Ref:       res.Ref,
					MountType: pb.MountType_BIND,
				},
			},
		})
		if err != nil {
			return res, err
		}

		defer ctr.Release(ctx)
		pid, err := ctr.Start(ctx, gwClient.StartRequest{
			Env: []string{
				// // "CNB_USER_ID=1002",
				// // "CNB_GROUP_ID=1000",
				"CNB_PLATFORM_API=0.12",
				"CNB_STACK_ID=io.buildpacks.stacks.jammy.tiny",
				// // "BP_PROCFILE_DEFAULT_PROCESS=web",
				// "BP_IMAGE_LABELS=alpha=bravo charlie=\"delta echo\"",
				// "BP_OCI_AUTHORS=paketo",
				// "BP_OCI_CREATED=2024-05-17T00:49:01Z",
				// "BP_OCI_DESCRIPTION=distroless-like jammy",
				// "BP_OCI_DOCUMENTATION=docs",
				// "BP_OCI_LICENSES=MIT",
				// "BP_OCI_REF_NAME=ttl.sh/wygin/buildkit:1d",
				// "BP_OCI_REVISION=1",
				// "BP_OCI_SOURCE=src",
				// "BP_OCI_TITLE=buildkit-test",
				// "BP_OCI_URL=https://github.com/paketo-buildpacks/jammy-tiny-stack",
				// "BP_OCI_VENDOR=Paketo Buildpacks",
				// "BP_OCI_VERSION=22.04",
			},
			// User: "root",
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Args: /*[]string{"ls", "-la", "workspace"},*/ []string{"/cnb/lifecycle/creator", "-app", "/workspace", "-run-image", "index.docker.io/paketobuildpacks/run-jammy-tiny:latest", "ttl.sh/wygin/react-yarn:1d"},
		})

		if err := pid.Wait(); err != nil {
			if err := pid.Signal(ctx, syscall.SIGKILL); err != nil {
				l.logger.Warn("test container failed to kill")
			}
			return res, err
		}

		return res, err
	}, progresswriter.ResetTime(mw.WithPrefix("test: ", true)).Status())
	if err != nil {
		return err
	}

	// l.UID(fmt.Sprintf("%d", l.opts.Builder.UID())).
	// 	GID(fmt.Sprintf("%d", l.opts.Builder.GID())).
	// 	AppSource(l.opts.AppPath, mounter.AppDir())

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
