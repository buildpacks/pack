package builder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/containerd/platforms"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/util/entitlements"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// Build solves the state and exports it to host filesystem
//
// The exported format can be one of the following
// - OCI tar file
// - Docker tar file
// - Image to registry
func (b *Builder[any]) Build(ctx context.Context) error {
	var statusChan chan *client.SolveStatus
	res, err := b.client.Build(ctx, client.SolveOpt{
		AllowedEntitlements: []entitlements.Entitlement{
			entitlements.EntitlementNetworkHost,
		},
		CacheExports: []client.CacheOptionsEntry{},
		CacheImports: []client.CacheOptionsEntry{},
		Exports: []client.ExportEntry{},
		Internal: true,
	}, "packerfile.v0", b.build, statusChan)
	if err != nil {
		return err
	}

	digest := res.ExporterResponse[exptypes.ExporterConfigDigestKey]
	fmt.Printf("successfully built image %s(%s)", b.ref, digest)
	return nil
}

// build solve the state and return the Result
func (b *Builder[any])build(ctx context.Context, c gatewayClient.Client) (*gatewayClient.Result, error) {
	if l := len(b.platforms); l > 1 { // multi-arch
		res := gatewayClient.NewResult() // empty result
		res.AddMeta("image.name", []byte(b.ref)) // added an annotation to the image/index manifest
		return b.multiArchBuild(ctx, c, res)
	} else if l == 0 {
		b.platforms = append(b.platforms, platforms.DefaultSpec()) // target current platform
	}

	p := b.platforms[0]
	def, err := b.State().Marshal(ctx, llb.Platform(p))
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal state")
	}

	res, err := c.Solve(ctx, gatewayClient.SolveRequest{
		Evaluate: true,
		// CacheImports: b.cacheImports, // TODO: update cache imports to [pack home]
		Definition:   def.ToPB(),
	})
	if err != nil {
		return res, errors.Wrap(err, "failed to solve")
	}

	ref, err := res.SingleRef()
	if err != nil {
		return res, err
	}

	_, err = ref.ToState()
	if err != nil {
		return res, err
	}

	res.SetRef(ref)

	config := b.ConfigFile()
	MutateConfigFile(config, p)
	configBytes, err := json.Marshal(config)
	if err != nil {
		return res, err
	}

	res.AddMeta(exptypes.ExporterImageConfigKey, configBytes)
	if b.prevImage != nil {
		baseConfig := b.prevImage.ConfigFile()
		MutateConfigFile(baseConfig, p)
		configBytes, err := json.Marshal(baseConfig)
		if err != nil {
			return res, err
		}
		res.AddMeta(exptypes.ExporterImageBaseConfigKey, configBytes)
	}
	return res, nil
}

// responsible for converting [llb.State] into multi-arch supported [gatewayClient.Result]
func (b *Builder[any]) multiArchBuild(ctx context.Context, c gatewayClient.Client, res *gatewayClient.Result) (*gatewayClient.Result, error) {
	expPlatforms := &exptypes.Platforms{
		Platforms: make([]exptypes.Platform, 0, len(b.platforms)),
	}

	eg, ctx1 := errgroup.WithContext(ctx)
	for i, p := range b.platforms {
		i, p := i, p
		eg.Go(func() error {
			def, err := b.State().Marshal(ctx1, llb.Platform(p))
			if err != nil {
				return errors.Wrap(err, "failed to marshal state")
			}

			r, err := c.Solve(ctx1, gatewayClient.SolveRequest{
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

			config := b.ConfigFile()
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
	return res, err
}

// adds platform to config
func MutateConfigFile(config *v1.ConfigFile, platform ocispecs.Platform) {
	config.OS = platform.OS
	config.Architecture = platform.Architecture
	config.Variant = platform.Variant
	config.OSVersion = platform.OSVersion
	config.OSFeatures = platform.OSFeatures
}
