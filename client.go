package pack

import (
	"path/filepath"

	"github.com/docker/docker/client"

	"github.com/buildpack/pack/lifecycle"

	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
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

func DefaultClient(config *config.Config, logger logging.Logger) (*Client, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, err
	}

	downloader := NewDownloader(logger, filepath.Join(config.Path(), "download-cache"))

	return &Client{
		config:           config,
		logger:           logger,
		imageFetcher:     image.NewFetcher(logger, dockerClient),
		buildpackFetcher: buildpack.NewFetcher(downloader),
		lifecycleFetcher: lifecycle.NewFetcher(downloader),
		lifecycle:        build.NewLifecycle(dockerClient, logger),
		docker:           dockerClient,
	}, nil
}
