package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/containerd/containerd/platforms"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/pack/internal/buildkit/state"
)

type Builder struct {
	state        state.State
	mounts       []client.Mount
	cacheImports []client.CacheOptionsEntry
	platforms    []ocispecs.Platform
	imageName    string
	prevImage    *state.State
}

func New(imageName string, state state.State, prevImage *state.State, platforms []ocispecs.Platform, mounts []client.Mount, imports []client.CacheOptionsEntry) *Builder {
	return &Builder{
		imageName:    imageName,
		state:        state,
		mounts:       mounts,
		cacheImports: imports,
		platforms:    platforms,
		prevImage:    prevImage,
	}
}

func (b *Builder) Build(ctx context.Context, c client.Client) (*client.Result, error) {
	res := client.NewResult()
	expPlatforms := &exptypes.Platforms{
		Platforms: make([]exptypes.Platform, 0, len(b.platforms)),
	}

	res.AddMeta("image.name", []byte(b.imageName))
	eg, ctx := errgroup.WithContext(ctx)
	for i, platform := range b.platforms {
		i, platform := i, platform
		eg.Go(func() error {
			def, err := b.state.Marshal(ctx, llb.Platform(platform))
			if err != nil {
				return errors.Wrap(err, "failed to marshal state")
			}

			r, err := c.Solve(ctx, client.SolveRequest{
				CacheImports: b.cacheImports,
				Definition:   def.ToPB(),
			})
			if err != nil {
				return errors.Wrap(err, "failed to solve")
			}

			ref, err := r.SingleRef()
			if err != nil {
				return err
			}

			_, err = ref.ToState()
			if err != nil {
				return err
			}

			p := platforms.Format(platform)
			res.AddRef(p, ref)

			config := b.state.ConfigFile()
			mutateConfigFile(&config, platform)
			configBytes, err := json.Marshal(config)
			if err != nil {
				return err
			}

			res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageConfigKey, p), configBytes)
			if b.prevImage != nil {
				baseConfig := b.prevImage.ConfigFile()
				mutateConfigFile(&baseConfig, platform)
				configBytes, err := json.Marshal(baseConfig)
				if err != nil {
					return err
				}
				res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageBaseConfigKey, p), configBytes)
			}

			expPlatforms.Platforms[i] = exptypes.Platform{
				ID:       p,
				Platform: platform,
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	dt, err := json.Marshal(expPlatforms)
	if err != nil {
		return nil, err
	}
	res.AddMeta(exptypes.ExporterPlatformsKey, dt)

	ctr, err := c.NewContainer(ctx, client.NewContainerRequest{
		Mounts: b.mounts,
	})
	if err != nil {
		return res, err
	}

	pid, err := ctr.Start(ctx, client.StartRequest{
		Args:   b.state.ConfigFile().Config.Cmd,
		Env:    b.state.ConfigFile().Config.Env,
		User:   b.state.ConfigFile().Config.User,
		Cwd:    b.state.ConfigFile().Config.WorkingDir,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		return res, err
	}

	if err := pid.Wait(); err != nil {
		return res, err
	}

	return res, ctr.Release(ctx)
}

func mutateConfigFile(config *v1.ConfigFile, platform ocispecs.Platform) {
	config.OS = platform.OS
	config.Architecture = platform.Architecture
	config.Variant = platform.Variant
	config.OSVersion = platform.OSVersion
	config.OSFeatures = platform.OSFeatures
}
