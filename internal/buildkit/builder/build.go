package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/containerd/containerd/platforms"
	"github.com/docker/cli/cli/config"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gwClient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	"github.com/moby/buildkit/util/progress/progresswriter"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Build solves the state and exports it to host filesystem
//
// The exported format can be one of the following
// - OCI tar file
// - Docker tar file
// - Image to registry
func (b *Builder[T]) Build(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	pw, err := progresswriter.NewPrinter(context.TODO(), os.Stderr, "plain")
	if err != nil {
		return err
	}
	mw := progresswriter.NewMultiWriter(pw)
	dockerConfig := config.LoadDefaultConfigFile(os.Stderr)
	// tlsConfigs, err := build.ParseRegistryAuthTLSContext(clicontext.StringSlice("registry-auth-tlscontext"))
	// if err != nil {
	// 	return err
	// }
	
	attachable := []session.Attachable{authprovider.NewDockerAuthProvider(dockerConfig, nil)}
	eg.Go(func() error {
		res, err := b.client.Build(ctx, client.SolveOpt{
			AllowedEntitlements: []entitlements.Entitlement{
				entitlements.EntitlementNetworkHost,
				entitlements.EntitlementSecurityInsecure,
			},
			CacheExports: []client.CacheOptionsEntry{},
			CacheImports: []client.CacheOptionsEntry{},
			Exports: []client.ExportEntry{},
			Session: attachable,
			// Internal: true,
		}, "packerfile.v0", b.build, progresswriter.ResetTime(mw.WithPrefix("packerfile.v0: ", true)).Status())
		if err != nil {
			return err
		}

		digest := res.ExporterResponse[exptypes.ExporterConfigDigestKey]
		fmt.Printf("successfully built image %s(%s)", b.ref, digest)

		return nil
	})

	eg.Go(func() error {
		<-pw.Done()
		return pw.Err()
	})

	return eg.Wait()
}

// build solve the state and return the Result
func (b *Builder[T])build(ctx context.Context, c gwClient.Client) (*gwClient.Result, error) {
	if l := len(b.platforms); l > 1 { // multi-arch
		res := gwClient.NewResult() // empty result
		res.AddMeta("image.name", []byte(b.ref))
		return b.multiArchBuild(ctx, c, res)
	} else if l == 0 {
		b.platforms = append(b.platforms, platforms.DefaultSpec()) // target current platform
	}

	// we need to create a single image with default target's platform
	p := b.platforms[0] 
	def, err := b.State().Marshal(ctx, llb.Platform(p))
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal state")
	}

	if len(def.Def) == 0 {
		return nil, errors.Errorf("empty definition sent to build")
	}

	res, err := c.Solve(ctx, gwClient.SolveRequest{
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
	
	return res, b.bootstrapContainer(ctx, c)
}

// responsible for converting [llb.State] into multi-arch supported [gwClient.Result]
func (b *Builder[any]) multiArchBuild(ctx context.Context, c gwClient.Client, res *gwClient.Result) (*gwClient.Result, error) {
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

			if len(def.Def) == 0 {
				return errors.Errorf("empty definition sent to build")
			}

			r, err := c.Solve(ctx1, gwClient.SolveRequest{
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

	return res, b.bootstrapMultiArchContainer(ctx, c)
}

func (b *Builder[T]) bootstrapMultiArchContainer(ctx context.Context, c gwClient.Client) error {
	workerID, platform, err := b.workerSupportCtr(c)
	if err != nil {
		return err
	}

	fmt.Printf("creating new container with platform: %s, worker: %s", platform.String(), workerID)
	hostPlatform := pb.PlatformFromSpec(platforms.DefaultSpec()) // same as platform but ocispec.Platform instead of v1.Platform
	ctr, err := c.NewContainer(ctx, gwClient.NewContainerRequest{
		Platform: &hostPlatform,
		NetMode: llb.NetModeHost, // ephemeral builder runs on `--network=host`
	})
	if err != nil {
		return err
	}

	defer ctr.Release(ctx)
	pid, err := ctr.Start(ctx, gwClient.StartRequest{})
	if err != nil {
		return err
	}

	doneCh := make(chan struct{})
	defer close(doneCh)
	go func() {
		select {
		case <-ctx.Done():
			if err := pid.Signal(ctx, syscall.SIGKILL); err != nil {
				logrus.Warnf("failed to kill process: %v", err)
			}
		case <-doneCh:
		}
	}()

	return pid.Wait()
}

// Lists the Platforms that are supported by the worker
// Returns an error if no supported worker supports the host's platform
func (b *Builder[T]) workerSupportCtr(c gwClient.Client) (workerID string, platform *v1.Platform, err error) {
	hostv1Platform := platforms.DefaultSpec() // returns the host's platform
	v1Platform, err := v1.ParsePlatform(platforms.Format(hostv1Platform))
	if err != nil {
		return workerID, v1Platform, err
	}

	var platformSupportedByWorker bool
	for _, w := range c.BuildOpts().Workers {
		for _, p := range w.Platforms {
			p, err := v1.ParsePlatform(platforms.Format(p))
			if err != nil {
				return workerID, v1Platform, err
			}

			// worker platform list might not contain the OSFeatures and Features
			// v1Platform#Satisifies ensures to ignore these fields!
			// use p#Satisifies iff worker platform cnatins OSFeatures and/or Features
			if v1Platform.Satisfies(*p) {
				platformSupportedByWorker = true
				workerID = w.ID
				break
			}
		}
	}
	if !platformSupportedByWorker {
		return workerID, v1Platform, errors.Errorf("platform %s is not supported by workers", v1Platform.String())
	}

	return workerID, v1Platform, nil
}

// creates a new conatiner and starts the default process
// 
func (b *Builder[T]) bootstrapContainer(ctx context.Context, c gwClient.Client) error {
	eg, ctx := errgroup.WithContext(ctx)
	ctrCtx, ctrCancel := context.WithCancel(ctx)
	defer ctrCancel()
	workerID, platform, err := b.workerSupportCtr(c)
	if err != nil {
		return err
	}

	eg.Go(func() error {
		fmt.Printf("creating new container with platform: %s, worker: %s\n", platform.String(), workerID)
		ctr, err := c.NewContainer(ctrCtx, gwClient.NewContainerRequest{
			NetMode: llb.NetModeHost, // ephemeral builder runs on `--network=host`
		})
		if err != nil {
			return err
		}
	
		defer ctr.Release(ctrCtx)
		fmt.Printf("starting container: \n %+v\n", ctr)
		pid, err := ctr.Start(ctrCtx, gwClient.StartRequest{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			SecurityMode: llb.SecurityModeInsecure,
			RemoveMountStubsRecursive: true,
		})
		doneCh := make(chan struct{})
		defer close(doneCh)
		go func() {
			select {
			case <-ctrCtx.Done():
				if err := pid.Signal(ctrCtx, syscall.SIGKILL); err != nil {
					logrus.Warnf("failed to kill process: %v", err)
				}
				logrus.Info("killed process")
			case <-doneCh:
			}
		}()
		if err != nil {
			return err
		}

		return pid.Wait()
	})

	return eg.Wait()
}

// adds platform to config
func MutateConfigFile(config *v1.ConfigFile, platform ocispecs.Platform) {
	config.OS = platform.OS
	config.Architecture = platform.Architecture
	config.Variant = platform.Variant
	config.OSVersion = platform.OSVersion
	config.OSFeatures = platform.OSFeatures
}
