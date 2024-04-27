package builder

import (
	"context"
	"os"

	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/buildkit/state"
)

type Builder struct {
	state.State
	Mounts       []client.Mount
	CacheImports []client.CacheOptionsEntry
}

func New(state state.State, mounts []client.Mount, imports []client.CacheOptionsEntry) *Builder {
	return &Builder{
		State:        state,
		Mounts:       mounts,
		CacheImports: imports,
	}
}

func (b *Builder) Build(ctx context.Context, c client.Client) (*client.Result, error) {
	def, err := b.State.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal state")
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		CacheImports: b.CacheImports,
		Definition:   def.ToPB(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to solve")
	}

	ctr, err := c.NewContainer(ctx, client.NewContainerRequest{
		Mounts: b.Mounts,
	})
	if err != nil {
		return res, err
	}

	pid, err := ctr.Start(ctx, client.StartRequest{
		Args:   b.ConfigFile().Config.Cmd,
		Env:    b.ConfigFile().Config.Env,
		User:   b.ConfigFile().Config.User,
		Cwd:    b.ConfigFile().Config.WorkingDir,
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
