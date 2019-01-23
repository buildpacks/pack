package containers

import (
	"context"

	dockertypes "github.com/docker/docker/api/types"
)

type Docker interface {
	ContainerRemove(ctx context.Context, containerID string, options dockertypes.ContainerRemoveOptions) error
}

func Remove(cli Docker, containerID string) error {
	return cli.ContainerRemove(context.Background(), containerID, dockertypes.ContainerRemoveOptions{Force: true})
}
