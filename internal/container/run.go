package container

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/term"
	"github.com/pkg/errors"
)

func Run(ctx context.Context, docker *client.Client, containerID string, out, errOut io.Writer) error {
	bodyChan, errChan := docker.ContainerWait(ctx, containerID, dcontainer.WaitConditionNextExit)

	if err := docker.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "container start")
	}
	logs, err := docker.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stdout")
	}

	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(out, errOut, logs)
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

func Run2(ctx context.Context, docker *client.Client, containerID string) error {
	fmt.Println("STARTING...")
	if err := docker.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
		return errors.Wrap(err, "container start")
	}
	fmt.Println("STARTED:", containerID)

	return nil
}

func RunExec(ctx context.Context, docker *client.Client, containerID, execID string, out, errOut io.Writer) error {
	//bodyChan, errChan := docker.ContainerWait(ctx, containerID, dcontainer.WaitConditionNextExit)

	fmt.Println("STARTING:LOGS")
	logs, err := docker.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return errors.Wrap(err, "container logs stdout")
	}

	copyErr := make(chan error)
	go func() {
		_, err := stdcopy.StdCopy(out, errOut, logs)
		copyErr <- err
	}()

	fmt.Println("STARTING:EXEC")
	if err := docker.ContainerExecStart(ctx, execID, types.ExecStartCheck{}); err != nil {
		return errors.Wrap(err, "container start")
	}

	// TODO: inspect exec for exit
	//inspect, err := docker.ContainerExecInspect(ctx, execID)

	//select {
	//case body := <-bodyChan:
	//	if body.StatusCode != 0 {
	//		return fmt.Errorf("failed with status code: %d", body.StatusCode)
	//	}
	//case err := <-errChan:
	//	return err
	//}
	return <-copyErr
}

func Start(ctx context.Context, client *client.Client, containerID string, hostConfig types.ContainerStartOptions) error {
	var (
		out io.Writer = os.Stdout
		err error
	)

	bodyChan, errChan := client.ContainerWait(ctx, containerID, dcontainer.WaitConditionNextExit)

	if _, ok := out.(*os.File); !ok {
		return errors.New("not a terminal")
	}

	// Attach to the container on a separate thread
	attachCtx, cancelFn := context.WithCancel(ctx)
	attachErrorChan := attachToContainer(attachCtx, client, containerID)
	defer cancelFn()

	// Start it
	err = client.ContainerStart(ctx, containerID, hostConfig)
	if err != nil {
		return err
	}

	// Make sure terminal resizes are passed on to the container
	//monitorTty(ctx, client, containerID, terminalFd)

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

func StartExec(ctx context.Context, client *client.Client, execID string) (err error) {
	var (
		terminalFd uintptr
		oldState   *term.State
		out        io.Writer = os.Stdout
	)

	if file, ok := out.(*os.File); ok {
		terminalFd = file.Fd()
	} else {
		return errors.New("Not a terminal!")
	}

	// Set up the pseudo terminal
	oldState, err = term.SetRawTerminal(terminalFd)
	if err != nil {
		return
	}

	// Clean up after the exec command has exited
	defer term.RestoreTerminal(terminalFd, oldState)

	// Start it
	errorChan := make(chan error)
	go startExec(ctx, client, execID, errorChan)

	// Make sure terminal resizes are passed on to the exec Tty
	monitorExecTty(ctx, client, execID, terminalFd)

	return <-errorChan
}

func attachToContainer(ctx context.Context, client *client.Client, containerID string) chan error {
	errChan := make(chan error)

	attached, err := client.ContainerAttach(ctx, containerID, types.ContainerAttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})

	if err != nil {
		errChan <- err
		return errChan
	}

	go io.Copy(os.Stdout, attached.Reader)
	go io.Copy(os.Stderr, attached.Reader)

	input := make(chan []byte)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for ctx.Err() == nil && scanner.Scan() {
			input <- []byte(scanner.Text())
		}
	}()

	// Write to docker container
	go func(w io.WriteCloser) {
		for ctx.Err() == nil {
			data, ok := <-input
			if !ok {
				errChan <- errors.New("failed to get input")
				return
			}

			_, err := w.Write(append(data, '\n'))
			if err != nil {
				errChan <- err
			}
		}
	}(attached.Conn)

	return errChan
}

func startExec(ctx context.Context, client *client.Client, execID string, errorChan chan error) {
	err := client.ContainerExecStart(ctx, execID, types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	})

	errorChan <- err
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

// From https://github.com/docker/docker/blob/0d70706b4b6bf9d5a5daf46dd147ca71270d0ab7/api/client/utils.go#L222-L233
func monitorExecTty(ctx context.Context, client *client.Client, execID string, terminalFd uintptr) {
	// HACK: For some weird reason on Docker 1.4.1 this resize is being triggered
	//       before the Exec instance is running resulting in an error on the
	//       Docker server. So we wait a little bit before triggering this first
	//       resize
	time.Sleep(50 * time.Millisecond)
	resizeExecTty(ctx, client, execID, terminalFd)

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			resizeExecTty(ctx, client, execID, terminalFd)
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

func resizeExecTty(ctx context.Context, client *client.Client, containerID string, terminalFd uintptr) error {
	height, width := getTtySize(terminalFd)
	if height == 0 && width == 0 {
		return nil
	}
	return client.ContainerExecResize(ctx, containerID, types.ResizeOptions{
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
