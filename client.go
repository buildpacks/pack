package pack

import (
	"os"
	"path/filepath"

	dockerClient "github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/lifecycle"
	"github.com/buildpack/pack/logging"
)

type Client struct {
	logger           logging.Logger
	imageFetcher     ImageFetcher
	buildpackFetcher BuildpackFetcher
	lifecycleFetcher LifecycleFetcher
	lifecycle        Lifecycle
	docker           *dockerClient.Client
}

type ClientOption func(c *Client)

// WithLogger supply your own logger.
func WithLogger(l logging.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithDockerClient supply your own docker client.
func WithDockerClient(docker *dockerClient.Client) ClientOption {
	return func(c *Client) {
		c.docker = docker
	}
}

func NewClient(opts ...ClientOption) (*Client, error) {
	var client Client

	for _, opt := range opts {
		opt(&client)
	}

	if client.logger == nil {
		client.logger = logging.New(os.Stderr)
	}

	if client.docker == nil {
		var err error
		client.docker, err = dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithVersion("1.38"))
		if err != nil {
			return nil, err
		}
	}

	packHome, err := config.PackHome()
	if err != nil {
		return nil, errors.Wrap(err, "getting pack home")
	}
	downloader := NewDownloader(client.logger, filepath.Join(packHome, "download-cache"))
	client.imageFetcher = image.NewFetcher(client.logger, client.docker)
	client.buildpackFetcher = buildpack.NewFetcher(downloader)
	client.lifecycleFetcher = lifecycle.NewFetcher(downloader)
	client.lifecycle = build.NewLifecycle(client.docker, client.logger)

	return &client, nil
}
