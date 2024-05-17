package builder

import (
	"context"

	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	"github.com/moby/buildkit/client"
	gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

func New[T any](ctx context.Context, ref string, state packerfile.Packerfile[T], mounts ...gatewayClient.Mount) (*Builder[T], error) {
	c, err := client.New(ctx, "")
	return &Builder[T]{
		ref: ref,
		Packerfile: state,
		mounts: mounts,
		entrypoint: make([]string, 1),
		cmd: make([]cmd, 0),
		envs: make([]string, 4),
		platforms: make([]ocispecs.Platform, 0),

		// defaults
		workdir: "/workspace",
		client: c,
	}, err
}

func CMD(path string) cmd {
	return cmd{
		path: path,
	}
}
