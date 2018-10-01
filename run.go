package pack

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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
	var err error
	r.AppDir, err = filepath.Abs(r.AppDir)
	if err != nil {
		return err
	}

	h := md5.New()
	io.WriteString(h, r.AppDir)
	repoName := fmt.Sprintf("%x", h.Sum(nil))

	r.Build = BuildFlags{
		AppDir:   r.AppDir,
		Builder:  r.Builder,
		RunImage: r.RunImage,
		RepoName: repoName,
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
	ctr, err := r.Build.Cli.ContainerCreate(ctx, &container.Config{
		Image:        r.Build.RepoName,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: nat.PortSet{
			"8080/tcp": struct{}{},
		},
	}, &container.HostConfig{
		AutoRemove: true,
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{
				{HostIP: "127.0.0.1", HostPort: fmt.Sprintf("%s/tcp", r.Port)},
			},
		},
	}, nil, "")

	// TODO cleanup signal flow
	var stopped bool
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			d := time.Duration(5) * time.Second
			stopped = true
			r.Build.Cli.ContainerStop(ctx, ctr.ID, &d)
		}
	}()

	if err = r.Build.Cli.RunContainer(ctx, ctr.ID, r.Build.Stdout, r.Build.Stderr); err != nil && !stopped {
		return errors.Wrap(err, "run built container")
	}

	return nil
}
