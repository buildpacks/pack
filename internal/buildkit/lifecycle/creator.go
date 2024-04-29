package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/moby/buildkit/client"
	gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/buildpacks/pack/internal/buildkit/builder"
	"github.com/buildpacks/pack/internal/buildkit/cnb"
	mountpaths "github.com/buildpacks/pack/internal/buildkit/mount_paths"
	"github.com/buildpacks/pack/internal/buildkit/packerfile/options"
	"github.com/buildpacks/pack/internal/buildkit/state"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/pkg/dist"
)

func (l *LifecycleExecution) Create(ctx context.Context, c *client.Client) error {
	mounter := mountpaths.MountPathsForOS(l.state.Platform().OS, l.opts.Workspace)
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

	// switch buildCache.Type() {
	// case cache.Image:
	// 	flags = append(flags, "-cache-image", buildCache.Name())
	// 	l.state = l.state.AddVolume(l.opts.Volumes...)
	// case cache.Volume, cache.Bind:
	// 	l.state = l.state.AddVolume(append(l.opts.Volumes, fmt.Sprintf("%s:%s", buildCache.Name(), mounter.CacheDir()))...)
	// }

	if l.opts.CreationTime != nil && l.platformAPI.AtLeast("0.9") {
		l.state = l.state.AddEnv(sourceDateEpochEnv, strconv.Itoa(int(l.opts.CreationTime.Unix())))
	}

	projectMetadata, err := json.Marshal(l.opts.ProjectMetadata)
	if err != nil {
		return err
	}
	flags = append(l.withLogLevel(flags...), l.opts.Image.String())
	userPerm := fmt.Sprintf("%s:%s", strconv.Itoa(l.opts.Builder.UID()), strconv.Itoa(l.opts.Builder.GID()))
	l.state = l.state.Entrypoint("lifecycle/creator").
		Network(l.opts.Network).
		MkFile(mounter.ProjectPath(), fs.ModePerm, projectMetadata).
		// Use Add, cause: The AppPath can be either a directory or a tar file!
		// The [Add] command is responsible for extracting tar and fetching remote files!
		Add([]string{l.opts.AppPath}, mounter.AppDir(), options.ADD{Chown: userPerm, Chmod: userPerm, Link: true})
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

		// can we use SecretAsEnv, cause The ENV is exposed in ConfigFile whereas SecretAsEnv not! But are we using DinD?
		l.state = l.state.User(state.RootUser(l.state.Platform().OS)).AddEnv("CNB_REGISTRY_AUTH", authConfig)
	} else {
		// TODO: WithDaemonAccess(l.opts.DockerHost)
		// Maybe adding ExtraHost?

		flags = append(flags, "-daemon", "-launch-cache", mounter.LaunchCacheDir())
		// l.state = l.state.AddVolume(fmt.Sprintf("%s:%s", launchCache.Name(), mounter.LaunchCacheDir()))
	}
	l.state = l.state.Cmd(flags...)

	cacheImports := []client.CacheOptionsEntry{
		{
			Type: client.ExporterLocal,
			Attrs: map[string]string{
				"src": filepath.Join(".", "DinD", "cache"),
			},
		},
		// Also import cache from registry if not present
		// {
		// 	Type: "registry",
		// 	Attrs: map[string]string{
		// 		"ref": target,
		// 	},
		// },
	}

	gatewayCacheImport := make([]gatewayClient.CacheOptionsEntry, 0, len(cacheImports))
	for _, c := range cacheImports {
		gatewayCacheImport = append(gatewayCacheImport, gatewayClient.CacheOptionsEntry{
			Type:  c.Type,
			Attrs: c.Attrs,
		})
	}
	mounts := make([]gatewayClient.Mount, 0, len(l.opts.Volumes))
	for _, v := range l.opts.Volumes {
		mounts = append(mounts, gatewayClient.Mount{
			Dest:      v,
			MountType: pb.MountType_CACHE,
			CacheOpt:  &pb.CacheOpt{Sharing: pb.CacheSharingOpt_SHARED},
		})
	}
	var platforms = make([]v1.Platform, 0)
	for _, target := range l.targets {
		target.Range(ctx, func(t dist.Target) error {
			platforms = append(platforms, v1.Platform{
				OS:           t.OS,
				Architecture: t.Arch,
				Variant:      t.ArchVariant,
				// TODO: add more fields
			})
			return nil
		})
	}
	b := builder.New(l.opts.Image.Name(), l.state, nil, platforms, mounts, gatewayCacheImport)
	// I don't think we need to export DinD image!
	// Instead of using volumes for caching let's use CacheImports and CacheExports.
	//
	_, err = c.Build(ctx, client.SolveOpt{
		CacheExports: []client.CacheOptionsEntry{
			{
				Type: client.ExporterLocal,
				Attrs: map[string]string{
					"dest": filepath.Join(".", "DinD", "cache"),
				},
			},
		},
		CacheImports: cacheImports,
	}, "", b.Build, nil)
	if err != nil {
		return err
	}

	return nil
}
