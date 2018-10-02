package pack

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

func Run(appDir, buildImage, runImage string) error {
	r := &RunFlags{
		AppDir:   appDir,
		Builder:  buildImage,
		RunImage: runImage,
	}
	if err := r.Init(); err != nil {
		return err
	}
	return r.Run()
}

type RunFlags struct {
	AppDir   string
	Builder  string
	RunImage string
	Port     string
	// Below are set by init
	Build BuildFlags
}

func (r *RunFlags) Init() error {
	r.Build = BuildFlags{
		AppDir:   r.AppDir,
		Builder:  r.Builder,
		RunImage: r.RunImage,
		RepoName: r.repoName(),
		Publish:  false,
		NoPull:   false,
	}

	return r.Build.Init()
}

func (r *RunFlags) Run() error {
	ctx := context.Background()

	err := r.Build.Run()
	if err != nil {
		return err
	}

	fmt.Println("*** RUNNING:")
	exposedPorts, portBindings, err := parsePorts(r.Port)
	if err != nil {
		return err
	}
	ctr, err := r.Build.Cli.ContainerCreate(ctx, &container.Config{
		Image:        r.Build.RepoName,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: exposedPorts,
	}, &container.HostConfig{
		AutoRemove:   true,
		PortBindings: portBindings,
	}, nil, "")

	sigs := makeSignalChan()
	stopped := false
	go func() {
		<-sigs
		stopped = true
		d := time.Duration(5) * time.Second
		r.Build.Cli.ContainerStop(ctx, ctr.ID, &d)
	}()

	logContainerListening(portBindings)
	if err = r.Build.Cli.RunContainer(ctx, ctr.ID, r.Build.Stdout, r.Build.Stderr); err != nil && !stopped {
		return errors.Wrap(err, "run built container")
	}

	return nil
}

func (r *RunFlags) repoName() string {
	dir, _ := filepath.Abs(r.AppDir)
	// we can ignore errors here because they will be caught later by the Build command
	h := md5.New()
	io.WriteString(h, dir)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func makeSignalChan() <-chan os.Signal {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	return sigs
}

func parsePorts(port string) (nat.PortSet, nat.PortMap, error) {
	ports := strings.Split(port, ",")
	for i, p := range ports {
		p = strings.TrimSpace(p)
		if _, err := strconv.Atoi(p); err == nil {
			// default simple port to localhost and 8080 inside the container
			p = fmt.Sprintf("127.0.0.1:%s:8080/tcp", p)
		}
		ports[i] = p
	}

	return nat.ParsePortSpecs(ports)
}

func logContainerListening(portBindings nat.PortMap) {
	// TODO handle case with multiple ports, for now we assume you know what
	// you're doing and don't need guidance
	if len(portBindings) == 1 {
		for _, bindings := range portBindings {
			if len(bindings) == 1 {
				binding := bindings[0]
				host := binding.HostIP
				port := binding.HostPort
				if host == "127.0.0.1" {
					host = "localhost"
				}
				fmt.Printf("Starting container listening at http://%s:%s/\n", host, port)
			}
		}
	}
}
