// TODO: We should rename this file (and its test and test file) to avoid confusion with the one in the `commands` package

package pack

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/containers"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
)

// This interface same as BuildConfig
//go:generate mockgen -package mocks -destination mocks/build_runner.go github.com/buildpack/pack BuildRunner
type BuildRunner interface {
	Run(context.Context) error
}

type RunFlags struct {
	BuildFlags BuildFlags
	Ports      []string
}

type RunConfig struct {
	Ports []string
	Build BuildRunner
	// All below are from BuildConfig
	RepoName string
	Cli      Docker
	Logger   *logging.Logger
}

func (bf *BuildFactory) RunConfigFromFlags(ctx context.Context, f *RunFlags) (*RunConfig, error) {
	bc, err := bf.BuildConfigFromFlags(ctx, &f.BuildFlags)
	if err != nil {
		return nil, err
	}
	rc := &RunConfig{
		Build: bc,
		Ports: f.Ports,
		// All below are from BuildConfig
		RepoName: bc.RepoName,
		Cli:      bc.Cli,
		Logger:   bc.Logger,
	}

	return rc, nil
}

func Run(ctx context.Context, outWriter, errWriter io.Writer, appDir, buildImage, runImage string, ports []string) error {
	// TODO: Receive Cache and docker client as an argument of this function
	dockerClient, err := docker.New()
	if err != nil {
		return err
	}
	c, err := cache.New(runImage, dockerClient)
	if err != nil {
		return err
	}
	imageFactory, err := image.NewFactory(image.WithOutWriter(outWriter))
	if err != nil {
		return err
	}
	imageFetcher := &ImageFetcher{
		Factory: imageFactory,
		Docker:  dockerClient,
	}
	logger := logging.NewLogger(outWriter, errWriter, true, false)
	bf, err := DefaultBuildFactory(logger, c, dockerClient, imageFetcher)
	if err != nil {
		return err
	}
	r, err := bf.RunConfigFromFlags(ctx,
		&RunFlags{
			BuildFlags: BuildFlags{
				AppDir:   appDir,
				Builder:  buildImage,
				RunImage: runImage,
			},
			Ports: ports,
		})
	if err != nil {
		return err
	}
	return r.Run(ctx)
}

func (r *RunConfig) Run(ctx context.Context) error {
	err := r.Build.Run(ctx)
	if err != nil {
		return err
	}

	r.Logger.Verbose(style.Step("RUNNING"))
	if r.Ports == nil {
		r.Ports, err = r.exposedPorts(ctx, r.RepoName)
		if err != nil {
			return err
		}
	}
	exposedPorts, portBindings, err := parsePorts(r.Ports)
	if err != nil {
		return err
	}
	ctr, err := r.Cli.ContainerCreate(ctx, &container.Config{
		Image:        r.RepoName,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: exposedPorts,
		Labels:       map[string]string{"author": "pack"},
	}, &container.HostConfig{
		AutoRemove:   true,
		PortBindings: portBindings,
	}, nil, "")
	if err != nil {
		return err
	}
	defer containers.Remove(r.Cli, ctr.ID)

	logContainerListening(r.Logger, portBindings)
	if err = r.Cli.RunContainer(ctx, ctr.ID, r.Logger.VerboseWriter(), r.Logger.VerboseErrorWriter()); err != nil {
		return errors.Wrap(err, "run container")
	}

	return nil
}

func (r *RunConfig) exposedPorts(ctx context.Context, imageID string) ([]string, error) {
	i, _, err := r.Cli.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, err
	}
	var ports []string
	for port := range i.Config.ExposedPorts {
		ports = append(ports, port.Port())
	}
	return ports, nil
}

func parsePorts(ports []string) (nat.PortSet, nat.PortMap, error) {
	for i, p := range ports {
		p = strings.TrimSpace(p)
		if _, err := strconv.Atoi(p); err == nil {
			// default simple port to localhost and inside the container
			p = fmt.Sprintf("127.0.0.1:%s:%s/tcp", p, p)
		}
		ports[i] = p
	}

	return nat.ParsePortSpecs(ports)
}

func logContainerListening(logger *logging.Logger, portBindings nat.PortMap) {
	// TODO handle case with multiple ports, for now when there is more than
	// one port we assume you know what you're doing and don't need guidance
	if len(portBindings) == 1 {
		for _, bindings := range portBindings {
			if len(bindings) == 1 {
				binding := bindings[0]
				host := binding.HostIP
				port := binding.HostPort
				if host == "127.0.0.1" {
					host = "localhost"
				}
				// TODO the service may not be http based
				logger.Info("Starting container listening at http://%s:%s/\n", host, port)
			}
		}
	}
}
