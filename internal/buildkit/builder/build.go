package builder

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"os"
// 	"strings"

// 	"github.com/buildpacks/pack/internal/buildkit/state"
// 	"github.com/containerd/containerd/platforms"
// 	v1 "github.com/google/go-containerregistry/pkg/v1"
// 	"github.com/moby/buildkit/client/llb"
// 	"github.com/moby/buildkit/exporter/containerimage/exptypes"
// 	"github.com/moby/buildkit/frontend/gateway/client"
// 	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
// 	"github.com/pkg/errors"
// 	"golang.org/x/sync/errgroup"
// )

// type builder struct { // let's make the [builder] private so that no one annoyingly changes builder's embaded [state.State]
// 	ref string
// 	state.State
// 	mounts []client.Mount
// 	entrypoint []string
// 	cmd []string
// 	envs []string
// 	user string
// 	attachStdin, attachStdout, attachStderr bool
// 	platforms []ocispecs.Platform
// 	prevImage *state.State
// 	workdir string
// }

// func New(ref string, state state.State, mounts []client.Mount) *builder {
// 	return &builder{
// 		ref: ref,
// 		State:        state,
// 		mounts: mounts,
// 		entrypoint: make([]string, 1),
// 		cmd: make([]string, 0),
// 		envs: make([]string, 4),
// 		platforms: make([]ocispecs.Platform, 0),

// 		// defaults
// 		workdir: "/workspace",
// 	}
// }

// func (b *builder) Entrypoint(entrypoint ...string) {
// 	b.State = b.State.Entrypoint(entrypoint...)
// 	b.entrypoint = entrypoint
// } 

// func (b *builder) Cmd(cmd []string) {
// 	b.State = b.State.Cmd(cmd...)
// 	b.cmd = cmd
// }

// func (b *builder) AddEnv(env string) {
// 	k, v, _ := strings.Cut(env, "=")
// 	b.State = b.State.AddEnv(k, v)
// 	b.envs = append(b.envs, env)
// }

// func (b *builder) User(user string) {
// 	b.State = b.State.User(user)
// 	b.user = user
// }

// func (b *builder) Workdir(dir string) {
// 	// b.State = b.State // TODO: add workdir
// 	b.workdir = dir
// }

// func (b *builder) AddPlatform(platform ocispecs.Platform) {
// 	b.platforms = append(b.platforms, platform)
// }

// func (b *builder) Stdin() {
// 	b.attachStdin = true
// }

// func (b *builder) Stdout() {
// 	b.attachStdout = true
// }

// func (b *builder) Stderr() {
// 	b.attachStderr = true
// }

// func (b *builder) Build(ctx context.Context, c client.Client) (resp *client.Result, err error) {
// 	res := client.NewResult()
// 	expPlatforms := &exptypes.Platforms{
// 		Platforms: make([]exptypes.Platform, 1),
// 	}

// 	res.AddMeta("image.name", []byte(b.ref)) // added an annotation to the image/index manifest
// 	eg, ctx1 := errgroup.WithContext(ctx)

// 	for i, platform := range b.platforms {
// 		i, platform := i, platform
// 		eg.Go(func() error {
// 			def, err := b.State.State().Network(llb.NetModeHost).Marshal(ctx1, llb.Platform(platform))
// 			if err != nil {
// 				return errors.Wrap(err, "failed to marshal state")
// 			}

// 			r, err := c.Solve(ctx1, client.SolveRequest{
// 				// CacheImports: b.cacheImports, // TODO: update cache imports to [pack home]
// 				Definition:   def.ToPB(),
// 			})
// 			if err != nil {
// 				return errors.Wrap(err, "failed to solve")
// 			}

// 			ref, err := r.SingleRef()
// 			if err != nil {
// 				return err
// 			}

// 			_, err = ref.ToState()
// 			if err != nil {
// 				return err
// 			}

// 			p := platforms.Format(platform)
// 			res.AddRef(p, ref)
// 			fmt.Printf("\n formatted platform: %s\n", p)

// 			config := b.State.ConfigFile()
// 			MutateConfigFile(config, platform)
// 			configBytes, err := json.Marshal(config)
// 			if err != nil {
// 				return err
// 			}

// 			res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageConfigKey, p), configBytes)
// 			if b.prevImage != nil {
// 				baseConfig := b.prevImage.ConfigFile()
// 				MutateConfigFile(baseConfig, platform)
// 				configBytes, err := json.Marshal(baseConfig)
// 				if err != nil {
// 					return err
// 				}
// 				res.AddMeta(fmt.Sprintf("%s/%s", exptypes.ExporterImageBaseConfigKey, p), configBytes)
// 			}

// 			expPlatforms.Platforms[i] = exptypes.Platform{
// 				ID:       p,
// 				Platform: platform,
// 			}
// 			fmt.Printf("\n export platform at %d is %s/%s/%s\n", i, platform.OS, platform.Architecture, platform.Variant)

// 			return nil
// 		})
// 	}

// 	if err := eg.Wait(); err != nil {
// 		return nil, err
// 	}

// 	dt, err := json.Marshal(expPlatforms)
// 	if err != nil {
// 		return res, errors.Wrap(err, "failed to marshal the target platforms")
// 	}

// 	fmt.Printf("\n multi-arch export platform: %v", expPlatforms.Platforms)

// 	res.AddMeta(exptypes.ExporterPlatformsKey, dt)

// 	for _, m := range b.mounts {
// 		m.Ref = res.Ref
// 		// m.ResultID = // TODO: required to mount to the same conatiner's volume
// 	}

// 	fmt.Printf("\ncreating new conatiner for %s\n", b.ref)
// 	ctr, err := c.NewContainer(ctx, client.NewContainerRequest{
// 		// Hostname: "pack",
// 		// Mounts: b.mounts,
// 		// NetMode: llb.NetModeHost, // --network=host
// 		// Platform: &pb.Platform{}, // TODO: create a container with the host's platform
// 	})
// 	if err != nil {
// 		return res, err
// 	}

// 	defer ctr.Release(ctx) // it will clean up the temp file created for the container to get start

// 	stdout := bytes.NewBuffer(nil)
// 	stderr := bytes.NewBuffer(nil)

// 	req := client.StartRequest{
// 		Args: []string{"sleep", "10"},// append(b.entrypoint, b.cmd...), // append([]string{"/cnb/lifecycle/creator", "-app" "/workspace" "-cache-dir" "/cache" "-run-image" "ghcr.io/jericop/run-jammy:latest" "wygin/react-yarn"}, b.state.Options().BuildArgs...),
// 		// Env: b.envs,
// 		// User: b.user,
// 		Stdout: &nopCloser{stdout},
// 		Stderr: &nopCloser{stderr},
// 		// SecurityMode: llb.SecurityModeSandbox,
// 		// Tty: true,
// 		// Cwd: "/layers",
// 	}

// 	// req = b.attach(req) // similar to `--attach` flag for docker build 

// 	fmt.Printf("\nstarting newly created conatiner for %s\n", b.ref)
// 	defer func() (*client.Result, error) {
// 		if recover() != nil {
// 			return res, fmt.Errorf("%q", recover())
// 		}
// 		return res, nil
// 	}()
// 	pid, err := ctr.Start(ctx, req)
// 	if err != nil {
// 		fmt.Printf("\n startContainer stdout: %q \n", stdout.String())
// 		fmt.Printf("\n startConatiner stderr: %q \n", stderr.String())
// 		return res, errors.Wrap(err, "starting container failed")
// 	}

// 	fmt.Printf("\n starting new process in conatiner %s", b.ref)
// 	if err := pid.Wait(); err != nil {
// 		fmt.Printf("\n pid stdout: %q \n", stdout.String())
// 		fmt.Printf("\n pid stderr: %q \n", stderr.String())
// 		return res, errors.Wrap(err, "container process failed")
// 	}
// 	fmt.Printf("\n finished the new process in conatiner %s", b.ref)

// 	// var status = make(chan *client.SolveStatus)
// 	// resp, err := c.Solve(ctx, def, client.SolveOpt{
// 	// 	Exports: []client.ExportEntry{
// 	// 		{
// 	// 			Type: "image",
// 	// 			Attrs: map[string]string{
// 	// 				"push": "true",
// 	// 				"push-by-digest": "true",
// 	// 			},
// 	// 		},
// 	// 	},
// 	// 	CacheExports: []client.CacheOptionsEntry{
// 	// 		{
// 	// 			Type: "local",
// 	// 			Attrs: map[string]string{
// 	// 				"dest": filepath.Join("DinD", "cache"),
// 	// 			},
// 	// 		},
// 	// 	},
// 	// 	CacheImports: []client.CacheOptionsEntry{
// 	// 		{
// 	// 			Type: "local",
// 	// 			Attrs: map[string]string{
// 	// 				"src": filepath.Join("DinD", "cache"),
// 	// 			},
// 	// 		},
// 	// 	},
// 	// }, nil)

// 	// ctx2, cancel := context.WithCancel(context.TODO())
// 	// defer cancel()
// 	// printer, err := progress.NewPrinter(ctx2, os.Stderr, "plain")
// 	// if err != nil {
// 	// 	return res, err
// 	// }

// 	// select {
// 	// case status := <- status:
// 	// 	fmt.Printf("status: \n")
// 	// 	printer.Write(status)
// 	// default:
// 	// 	return resp, err
// 	// }
// 	return res, err
// }

// func (b *builder) attach(req client.StartRequest) client.StartRequest {
// 	configFile := b.State.ConfigFile()
// 	if b.attachStdin {
// 		configFile.Config.AttachStdin = true
// 		req.Stdin = os.Stdin
// 	}

// 	if b.attachStdout {
// 		configFile.Config.AttachStdout = true
// 		req.Stdout = os.Stdin
// 	}

// 	if b.attachStderr {
// 		configFile.Config.AttachStderr = true
// 		req.Stderr = os.Stderr
// 	}

// 	return req
// }

// func MutateConfigFile(config *v1.ConfigFile, platform ocispecs.Platform) {
// 	config.OS = platform.OS
// 	config.Architecture = platform.Architecture
// 	config.Variant = platform.Variant
// 	config.OSVersion = platform.OSVersion
// 	config.OSFeatures = platform.OSFeatures
// }

// type nopCloser struct {
// 	io.Writer
// }

// func (n *nopCloser) Close() error {
// 	return nil
// }
