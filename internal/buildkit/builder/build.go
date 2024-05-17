package builder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/containerd/platforms"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	"github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func (b *builder[any])Build(ctx context.Context, c client.Client) (*client.Result, error) {
	res := client.NewResult() // empty result
	expPlatforms := &exptypes.Platforms{
		Platforms: make([]exptypes.Platform, 0, len(b.platforms)),
	}

	res.AddMeta("image.name", []byte(b.ref)) // added an annotation to the image/index manifest
	eg, ctx1 := errgroup.WithContext(ctx)

	for i, p := range b.platforms {
		i, p := i, p
		eg.Go(func() error {
			def, err := b.State.State().Marshal(ctx1, llb.Platform(p))
			if err != nil {
				return errors.Wrap(err, "failed to marshal state")
			}

			r, err := c.Solve(ctx1, client.SolveRequest{
				Evaluate: true,
				// CacheImports: b.cacheImports, // TODO: update cache imports to [pack home]
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

			platform := platforms.Format(p)
			res.AddRef(platform, ref)

			config := b.State.ConfigFile()
			MutateConfigFile(config, p)
			configBytes, err := json.Marshal(config)
			if err != nil {
				return err
			}

			res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageConfigKey, platform), configBytes)
			if b.prevImage != nil {
				baseConfig := b.prevImage.ConfigFile()
				MutateConfigFile(baseConfig, p)
				configBytes, err := json.Marshal(baseConfig)
				if err != nil {
					return err
				}
				res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageBaseConfigKey, platform), configBytes)
			}

			expPlatforms.Platforms[i] = exptypes.Platform{
				ID:       platform,
				Platform: p,
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	dt, err := json.Marshal(expPlatforms)
	if err != nil {
		return res, errors.Wrap(err, "failed to marshal the target platforms")
	}

	res.AddMeta(exptypes.ExporterPlatformsKey, dt)
	for _, m := range b.mounts {
		m.Ref = res.Ref
	}

	return res, nil
}

func MutateConfigFile(config *v1.ConfigFile, platform ocispecs.Platform) {
	config.OS = platform.OS
	config.Architecture = platform.Architecture
	config.Variant = platform.Variant
	config.OSVersion = platform.OSVersion
	config.OSFeatures = platform.OSFeatures
}
