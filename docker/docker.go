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
	logs, err := d.ContainerLogs(ctx, id, dockertypes.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stdout")
	}
	go func() {
		header := make([]byte, 8)
		for {
			if n, err := logs.Read(header); err != nil || n != 8 {
				continue
			}
			if header[0] == uint8(1) {
				if _, err := io.CopyN(stdout, logs, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
					break
				}
			} else if header[0] == uint8(2) {
				if _, err := io.CopyN(stderr, logs, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
					break
				}
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
