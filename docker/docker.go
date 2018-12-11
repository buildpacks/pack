package docker

import (
	"context"
	"fmt"
	"io"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
)

type Client struct {
	*dockercli.Client
}

func New() (*Client, error) {
	cli, err := dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
	if err != nil {
		return nil, errors.Wrap(err, "new docker client")
	}
	return &Client{cli}, nil
}

func (d *Client) RunContainer(ctx context.Context, id string, stdout io.Writer, stderr io.Writer) error {
	bodyChan, errChan := d.ContainerWait(ctx, id, container.WaitConditionNextExit)

	if err := d.ContainerStart(ctx, id, dockertypes.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "container start")
	}
	logs, err := d.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stdout")
	}

	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(stdout, stderr, logs)
		copyErr <- err
	}()

	select {
	case body := <-bodyChan:
		if body.StatusCode != 0 {
			return fmt.Errorf("failed with status code: %d", body.StatusCode)
		}
	case err := <-errChan:
		fmt.Printf("ERR: %#v\n", err)
		return err
	}
	return <-copyErr
}
