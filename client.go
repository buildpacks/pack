package pack

import (
	"os"
	"path/filepath"

	"github.com/docker/docker/client"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/lifecycle"
	"github.com/buildpack/pack/logging"
)

type Client struct {
	config           *config.Config
	logger           logging.Logger
	imageFetcher     ImageFetcher
	buildpackFetcher BuildpackFetcher
	lifecycleFetcher LifecycleFetcher
	lifecycle        Lifecycle
	docker           *client.Client
}

func NewClient(
	config *config.Config,
	logger logging.Logger,
	imageFetcher ImageFetcher,
	buildpackFetcher BuildpackFetcher,
	lifecycleFetcher LifecycleFetcher,
	lifecycle Lifecycle,
	docker *client.Client,
) *Client {
	return &Client{
		config:           config,
		logger:           logger,
		imageFetcher:     imageFetcher,
		buildpackFetcher: buildpackFetcher,
		lifecycleFetcher: lifecycleFetcher,
		lifecycle:        lifecycle,
		docker:           docker,
	}
}

type ClientOption func(c *Client)

// WithLogger supply your own logger.
func WithLogger(l logging.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

func DefaultClient(config *config.Config, opts ...ClientOption) (*Client, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, err
	}

	var client Client

	for _, opt := range opts {
		opt(&client)
	}

	if client.logger == nil {
		client.logger = logging.New(os.Stderr)
	}

	downloader := NewDownloader(client.logger, filepath.Join(config.Path(), "download-cache"))
	client.config = config
	client.imageFetcher = image.NewFetcher(client.logger, dockerClient)
	client.buildpackFetcher = buildpack.NewFetcher(downloader)
	client.lifecycleFetcher = lifecycle.NewFetcher(downloader)
	client.lifecycle = build.NewLifecycle(dockerClient, client.logger)
	client.docker = dockerClient

	return &client, nil
}
