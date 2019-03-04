package dockerdial

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/hashicorp/yamux"
	"github.com/pkg/errors"
)

type dockerdial struct {
	ctrID   string
	session *yamux.Session
}

func new(stderr io.Writer) (*dockerdial, error) {
	dockerCli, err := dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
	if err != nil {
		return nil, errors.Wrap(err, "dockerdial: connect to docker:")
	}

	s := &dockerdial{}
	ctx := context.Background()

	pullBody, err := dockerCli.ImagePull(ctx, "dgodd/dockerdial:v1", dockertypes.ImagePullOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "dockerdial: pull image:")
	}
	io.Copy(ioutil.Discard, pullBody)
	pullBody.Close()

	ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
		Image:        "dgodd/dockerdial:v1",
		OpenStdin:    true,
		StdinOnce:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}, &container.HostConfig{
		AutoRemove:  true,
		NetworkMode: "host",
	}, nil, "")
	if err != nil {
		return nil, errors.Wrap(err, "dockerdial: create container:")
	}
	s.ctrID = ctr.ID

	res, err := dockerCli.ContainerAttach(ctx, ctr.ID, dockertypes.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		dockerCli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
		return nil, errors.Wrap(err, "dockerdial: attach:")
	}

	err = dockerCli.ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	if err != nil {
		dockerCli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
		return nil, errors.Wrap(err, "dockerdial: attach:")
	}

	pr, pw := io.Pipe()
	go stdcopy.StdCopy(pw, stderr, res.Reader)

	buf := make([]byte, 8)
	_, err = pr.Read(buf)
	if string(buf) != "STARTED\n" {
		res.Close()
		dockerCli.ContainerKill(ctx, ctr.ID, "SIGKILL")
		return nil, errors.New("dockerdial: did not read started")
	}

	s.session, err = yamux.Client(&StdinStdout{in: pr, out: res.Conn}, nil)
	if string(buf) != "STARTED\n" {
		res.Close()
		dockerCli.ContainerKill(ctx, ctr.ID, "SIGKILL")
		return nil, errors.New("dockerdial: create session")
	}

	return s, nil
}

var connOnce sync.Once
var connSingle *dockerdial
var connErr error

func Dial(network, addr string) (net.Conn, error) {
	// fmt.Printf("DIAL: |%s| - |%s|\n", network, addr)
	if network != "tcp" {
		return nil, fmt.Errorf("only tcp is implemented: %s", network)
	}

	connOnce.Do(func() {
		connSingle, connErr = new(ioutil.Discard)
	})
	if connErr != nil {
		return nil, errors.Wrap(connErr, "getting dial singleton")
	}

	c, err := connSingle.session.Open()
	if err != nil {
		return nil, err
	}

	addrLen := make([]byte, 4)
	binary.LittleEndian.PutUint32(addrLen, uint32(len(addr)))
	if _, err := c.Write(addrLen); err != nil {
		c.Close()
		return nil, err
	}
	if _, err := c.Write([]byte(addr)); err != nil {
		c.Close()
		return nil, err
	}

	return netConnFromReadWriteCloser{c}, nil
}

type StdinStdout struct {
	in  io.ReadCloser
	out io.WriteCloser
}

func (s *StdinStdout) Read(b []byte) (int, error) {
	return s.in.Read(b)
}
func (s *StdinStdout) Write(b []byte) (int, error) {
	return s.out.Write(b)
}
func (s *StdinStdout) Close() error {
	e1 := s.in.Close()
	e2 := s.out.Close()
	if e1 != nil {
		return e1
	}
	return e2
}

type netConnFromReadWriteCloser struct {
	io.ReadWriteCloser
}

func (netConnFromReadWriteCloser) LocalAddr() net.Addr {
	panic("not implemented")
}
func (netConnFromReadWriteCloser) RemoteAddr() net.Addr {
	panic("not implemented")
}
func (netConnFromReadWriteCloser) SetDeadline(t time.Time) error {
	panic("not implemented")
}
func (netConnFromReadWriteCloser) SetReadDeadline(t time.Time) error {
	panic("not implemented")
}
func (netConnFromReadWriteCloser) SetWriteDeadline(t time.Time) error {
	panic("not implemented")
}
