package builder

import (
	"context"

	"github.com/buildpacks/pack/internal/buildkit/packerfile"
	"github.com/moby/buildkit/client"
	gatewayClient "github.com/moby/buildkit/frontend/gateway/client"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

type builder[T any] struct { // let's make the [builder] private so that no one annoyingly changes builder's embaded [state.State]
	ref string // name of the builder
	State packerfile.Packerfile[T] // state of the builder
	client *client.Client // client to solve the state
	mounts []gatewayClient.Mount // mounts to mount to the container
	entrypoint []string // entrypoint of the container
	cmd []string
	envs []string
	user string
	attachStdin, attachStdout, attachStderr bool
	platforms []ocispecs.Platform
	prevImage *packerfile.Packerfile[T]
	workdir string
}

func New[T any](ctx context.Context, ref string, state packerfile.Packerfile[T], mounts ...gatewayClient.Mount) (*builder[T], error) {
	c, err := client.New(ctx, "")
	return &builder[T]{
		ref: ref,
		State:        state,
		mounts: mounts,
		entrypoint: make([]string, 1),
		cmd: make([]string, 0),
		envs: make([]string, 4),
		platforms: make([]ocispecs.Platform, 0),

		// defaults
		workdir: "/workspace",
		client: c,
	}, err
}