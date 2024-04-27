package executor

import (
	"context"
	"os"

	"github.com/moby/buildkit/client"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/buildkit/lifecycle"
)

func (l LifecycleExecutor) Execute(ctx context.Context, opts build.LifecycleOptions) error {
	tmpDir, err := os.MkdirTemp("", "pack.tmp")
	if err != nil {
		return err
	}

	exec, err := lifecycle.NewLifecycleExecution(l.logger, l.state, l.targets, tmpDir, opts)
	if err != nil {
		return err
	}

	client, err := client.New(ctx, "")
	if err != nil {
		return err
	}
	if !opts.Interactive {
		return exec.Create(ctx, client)
	}

	return opts.Termui.Run(func() {
		exec.Create(ctx, client)
	})
}
