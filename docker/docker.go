package docker

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/pkg/errors"
)

type Client struct {
	*dockercli.Client
}

func New() (*Client, error) {
	cli, err := dockercli.NewEnvClient()
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
	stdout2, err := d.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stdout")
	}
	go func() {
		for {
			header := make([]byte, 8)
			if n, err := stdout2.Read(header); err != nil || n != 8 {
				continue
			}
			if _, err := io.CopyN(stdout, stdout2, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
				break
			}
		}
	}()
	stderr2, err := d.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stderr")
	}
	go func() {
		for {
			header := make([]byte, 8)
			if n, err := stderr2.Read(header); err != nil || n != 8 {
				continue
			}
			if _, err := io.CopyN(stderr, stderr2, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
				break
			}
		}
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
	return nil
}

func (d *Client) PullImage(ref string) error {
	rc, err := d.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}
