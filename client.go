package pack

import (
	"github.com/docker/docker/client"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/logging"
)

type Client struct {
	config  *config.Config
	logger  *logging.Logger
	fetcher ImageFetcher
}

func NewClient(config *config.Config, logger *logging.Logger, fetcher ImageFetcher) *Client {
	return &Client{
		config:  config,
		logger:  logger,
		fetcher: fetcher,
	}
}

func DefaultClient(config *config.Config, logger *logging.Logger) (*Client, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, err
	}

	fetcher, err := image.NewFetcher(logger, dockerClient)
	if err != nil {
		return nil, err
	}

	return &Client{
		config:  config,
		logger:  logger,
		fetcher: fetcher,
	}, nil
}
