package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/api/types"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/term"
	"github.com/pkg/errors"
)

func Run(ctx context.Context, docker client.CommonAPIClient, containerID string, out, errOut io.Writer) error {
	bodyChan, errChan := docker.ContainerWait(ctx, containerID, dcontainer.WaitConditionNextExit)

	resp, err := docker.ContainerAttach(ctx, containerID, types.ContainerAttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	if err := docker.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "container start")
	}

	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(out, errOut, resp.Reader)

		copyErr <- err
	}()

	select {
	case body := <-bodyChan:
		if body.StatusCode != 0 {
			return fmt.Errorf("failed with status code: %d", body.StatusCode)
		}
	case err := <-errChan:
		return err
	}

	return <-copyErr
}

func Start(ctx context.Context, client client.CommonAPIClient, containerID string, hostConfig types.ContainerStartOptions) error {
	bodyChan, errChan := client.ContainerWait(ctx, containerID, dcontainer.WaitConditionNextExit)

	// Attach to the container on a separate thread
	attachCtx, cancelFn := context.WithCancel(ctx)
	defer cancelFn()

	readerCloser, err := attachToContainerReader(attachCtx, client, containerID)
	if err != nil {
		return errors.Wrap(err, "attach reader")
	}
	defer readerCloser.Close()

	err = client.ContainerStart(ctx, containerID, hostConfig)
	if err != nil {
		return errors.Wrap(err, "starting container")
	}

	// Start it
	writerCloser, attachErrorChan := attachToContainerWriter(attachCtx, os.Stdin, client, containerID)
	defer func() {
		fmt.Println("finalizing", containerID)
		writerCloser.Close()
	}()

	// TODO: wire and verify that this works
	// Make sure terminal resizes are passed on to the container
	// monitorTty(ctx, client, containerID, terminalFd)

	select {
	case err := <-attachErrorChan:
		fmt.Println("attach:err=", err)
		return err
	case body := <-bodyChan:
		fmt.Println("await:status=", body.StatusCode)
		if body.StatusCode != 0 {
			return fmt.Errorf("failed with status code: %d", body.StatusCode)
		}
	case err := <-errChan:
		fmt.Println("await:err=", err)
		return err
	}

	return nil
}

func attachToContainerReader(ctx context.Context, client client.CommonAPIClient, containerID string) (io.Closer, error) {
	attached, err := client.ContainerAttach(ctx, containerID, types.ContainerAttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  false,
		Stream: true,
	})

	if err != nil {
		return nil, err
	}

	go io.Copy(os.Stdout, attached.Reader)
	go io.Copy(os.Stderr, attached.Reader)

	return attached.Conn, nil
}

func attachToContainerWriter(ctx context.Context, stdIn io.Reader, client client.CommonAPIClient, containerID string) (io.Closer, chan error) {
	errChan := make(chan error)

	attached, err := client.ContainerAttach(ctx, containerID, types.ContainerAttachOptions{
		Stderr: false,
		Stdout: false,
		Stdin:  true,
		Stream: true,
	})

	if err != nil {
		errChan <- err
		return nil, errChan
	}

	go func(w io.WriteCloser) {
		fmt.Println("scan:pre", containerID)
		for ctx.Err() == nil {
			fmt.Println("scan:read", containerID)
			_, err := io.Copy(w, stdIn)

			fmt.Println("scan:post", containerID)
			if err != nil {
				errChan <- err
			}
		}
	}(attached.Conn)

	return attached.Conn, errChan
}

// From https://github.com/docker/docker/blob/0d70706b4b6bf9d5a5daf46dd147ca71270d0ab7/api/client/utils.go#L222-L233
func monitorTty(ctx context.Context, client *client.Client, containerID string, terminalFd uintptr) {
	resizeTty(ctx, client, containerID, terminalFd)

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			resizeTty(ctx, client, containerID, terminalFd)
		}
	}()
}

func resizeTty(ctx context.Context, client *client.Client, containerID string, terminalFd uintptr) error {
	height, width := getTtySize(terminalFd)
	if height == 0 && width == 0 {
		return nil
	}

	return client.ContainerResize(ctx, containerID, types.ResizeOptions{
		Height: height,
		Width:  width,
	})
}

// From https://github.com/docker/docker/blob/0d70706b4b6bf9d5a5daf46dd147ca71270d0ab7/api/client/utils.go#L235-L247
func getTtySize(terminalFd uintptr) (uint, uint) {
	ws, err := term.GetWinsize(terminalFd)
	if err != nil {
		return 0, 0
	}

	return uint(ws.Height), uint(ws.Width)
}
