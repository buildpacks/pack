package pack

import (
	"github.com/buildpack/lifecycle/image"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/logging"
)

type Client struct {
	config  *config.Config
	logger  *logging.Logger
	fetcher Fetcher
}

func NewClient(config *config.Config, logger *logging.Logger, fetcher Fetcher) *Client {
	return &Client{
		config:  config,
		logger:  logger,
		fetcher: fetcher,
	}
}

func DefaultClient(config *config.Config, logger *logging.Logger) (*Client, error) {
	factory, err := image.NewFactory()
	if err != nil {
		return nil, err
	}

	dockerClient, err := docker.New()
	if err != nil {
		return nil, err
	}

	fetcher := &ImageFetcher{
		Factory: factory,
		Docker:  dockerClient,
	}

	return &Client{
		config:  config,
		logger:  logger,
		fetcher: fetcher,
	}, nil
}
