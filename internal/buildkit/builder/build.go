package builder

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"

	"github.com/buildpacks/pack/internal/buildkit/state"
)

type Builder struct {
	ref string
	state        state.State
	mounts []string
}

func New(ref string, state state.State, mounts []string) *Builder {
	return &Builder{
		ref: ref,
		state:        state,
		mounts: mounts,
	}
}

func (b *Builder) Build(ctx context.Context, c client.Client) (res *client.Result, err error) {	
	def, err := b.state.State().Marshal(ctx)
	if err != nil {
		return nil, err
	}

	if err = llb.WriteTo(def, os.Stderr); err != nil {
		return nil, err
	}

	resp, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
		CacheImports: []client.CacheOptionsEntry{
			{
				Type: "local",
				Attrs: map[string]string{
					"src": filepath.Join("DinD", "cache"),
				},
			},
		},
	})
	if err != nil {
		return resp, err
	}

	// var mounts = make([]client.Mount, 0)
	// for _, m := range b.mounts {
	// 	mounts = append(mounts, client.Mount{
	// 		MountType: pb.MountType_CACHE,
	// 		CacheOpt: &pb.CacheOpt{Sharing: pb.CacheSharingOpt_SHARED},
	// 		Dest: m,
	// 		Ref: resp.Ref,
	// 	})
	// }

	// ctx2, cancel := context.WithCancel(context.TODO())
	// defer cancel()

	ctr, err := c.NewContainer(ctx, client.NewContainerRequest{
		// Mounts: mounts,
		// NetMode: llb.NetModeSandbox,
	})
	if err != nil {
		return resp, err
	}

	defer ctr.Release(ctx)

	pid, err := ctr.Start(ctx, client.StartRequest{
		// Stdin: os.Stdin,
		// Stdout: os.Stdout,
		// Stderr: os.Stderr,
		// Args: []string{"sleep", "10", "&&", "echo", "hello world!"}, // append([]string{"/cnb/lifecycle/creator", "-app" "/workspace" "-cache-dir" "/cache" "-run-image" "ghcr.io/jericop/run-jammy:latest" "wygin/react-yarn"}, b.state.Options().BuildArgs...),
		// Env: b.state.Options().Envs,
		// User: b.state.Options().User,
	})
	if err != nil {
		return resp, errors.New("starting container failed: " + err.Error())
	}

	if err := pid.Wait(); err != nil {
		return resp, errors.New("pid failed: " + err.Error())
	}

	// var status = make(chan *client.SolveStatus)
	// resp, err := c.Solve(ctx, def, client.SolveOpt{
	// 	Exports: []client.ExportEntry{
	// 		{
	// 			Type: "image",
	// 			Attrs: map[string]string{
	// 				"push": "true",
	// 				"push-by-digest": "true",
	// 			},
	// 		},
	// 	},
	// 	CacheExports: []client.CacheOptionsEntry{
	// 		{
	// 			Type: "local",
	// 			Attrs: map[string]string{
	// 				"dest": filepath.Join("DinD", "cache"),
	// 			},
	// 		},
	// 	},
	// 	CacheImports: []client.CacheOptionsEntry{
	// 		{
	// 			Type: "local",
	// 			Attrs: map[string]string{
	// 				"src": filepath.Join("DinD", "cache"),
	// 			},
	// 		},
	// 	},
	// }, nil)

	// ctx2, cancel := context.WithCancel(context.TODO())
	// defer cancel()
	// printer, err := progress.NewPrinter(ctx2, os.Stderr, "plain")
	// if err != nil {
	// 	return res, err
	// }

	// select {
	// case status := <- status:
	// 	fmt.Printf("status: \n")
	// 	printer.Write(status)
	// default:
	// 	return resp, err
	// }
	return resp, err
}
