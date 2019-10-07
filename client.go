package pack

import (
	"os"
	"path/filepath"

	"github.com/buildpack/imgutil"
	dockerClient "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/blob"
	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/logging"
)

type Client struct {
	logger       logging.Logger
	imageFetcher ImageFetcher
	downloader   Downloader
	lifecycle    Lifecycle
	docker       *dockerClient.Client
	imageFactory ImageFactory
}

type ClientOption func(c *Client)

// WithLogger supply your own logger.
func WithLogger(l logging.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// WithImageFactory supply your own image factory.
func WithImageFactory(f ImageFactory) ClientOption {
	return func(c *Client) {
		c.imageFactory = f
	}
}

// WithFetcher supply your own fetcher.
func WithFetcher(f ImageFetcher) ClientOption {
	return func(c *Client) {
		c.imageFetcher = f
	}
}

// WithDownloader supply your own downloader.
func WithDownloader(d Downloader) ClientOption {
	return func(c *Client) {
		c.downloader = d
	}
}

// WithCacheDir supply your own cache directory.
//
// Deprecated: use WithDownloader instead.
func WithCacheDir(path string) ClientOption {
	return func(c *Client) {
		c.downloader = blob.NewDownloader(c.logger, path)
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

	if client.downloader == nil {
		packHome, err := config.PackHome()
		if err != nil {
			return nil, errors.Wrap(err, "getting pack home")
		}
		client.downloader = blob.NewDownloader(client.logger, filepath.Join(packHome, "download-cache"))
	}

	if client.imageFetcher == nil {
		client.imageFetcher = image.NewFetcher(client.logger, client.docker)
	}

	if client.imageFactory == nil {
		client.imageFactory = &DefaultImageFactory{
			dockerClient: client.docker,
			keychain:     authn.DefaultKeychain,
		}
	}

	client.lifecycle = build.NewLifecycle(client.docker, client.logger)

	return &client, nil
}

type DefaultImageFactory struct {
	dockerClient *dockerClient.Client
	keychain     authn.Keychain
}

func (f *DefaultImageFactory) NewImage(repoName string, local bool) (imgutil.Image, error) {
	if local {
		return imgutil.EmptyLocalImage(repoName, f.dockerClient), nil
	}

	return imgutil.NewRemoteImage(repoName, f.keychain)
}
