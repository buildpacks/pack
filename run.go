package pack

import (
	"context"
	"io"

	"github.com/docker/docker/client"

	"github.com/buildpack/pack/app"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

// This interface same as BuildConfig
//go:generate mockgen -package mocks -destination mocks/build_runner.go github.com/buildpack/pack BuildRunner
type BuildRunner interface {
	Run(context.Context) (*app.Image, error)
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
		Logger:   bc.Logger,
	}

	return rc, nil
}

func Run(ctx context.Context, outWriter, errWriter io.Writer, appDir, buildImage, runImage string, ports []string) error {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return err
	}

	c, err := cache.New(runImage, dockerClient)
	if err != nil {
		return err
	}

	logger := logging.NewLogger(outWriter, errWriter, true, false)

	imageFetcher, err := image.NewFetcher(logger, dockerClient)
	if err != nil {
		return err
	}

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

	return r.Run(ctx, dockerClient)
}

func (r *RunConfig) Run(ctx context.Context, docker *client.Client) error {
	appImage, err := r.Build.Run(ctx)
	if err != nil {
		return err
	}

	r.Logger.Verbose(style.Step("RUNNING"))
	return appImage.Run(ctx, docker, r.Ports)
}
