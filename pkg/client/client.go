/*
Package client provides all the functionally provided by pack as a library through a go api.

# Prerequisites

In order to use most functionality, you will need an OCI runtime such as Docker or podman installed.

# References

This package provides functionality to create and manipulate all artifacts outlined in the Cloud Native Buildpacks specification.
An introduction to these artifacts and their usage can be found at https://buildpacks.io/docs/.

The formal specification of the pack platform provides can be found at: https://github.com/buildpacks/spec.
*/
package client

import (
	"context"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	dockerClient "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/build"
	iconfig "github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	runtime "github.com/buildpacks/pack/pkg/runtime"
)

//go:generate mockgen -package testmocks -destination ../testmocks/mock_docker_client.go github.com/docker/docker/client CommonAPIClient

//go:generate mockgen -package testmocks -destination ../testmocks/mock_image_fetcher.go github.com/buildpacks/pack/pkg/client ImageFetcher

// ImageFetcher is an interface representing the ability to fetch local and remote images.
type ImageFetcher interface {
	// Fetch fetches an image by resolving it both remotely and locally depending on provided parameters.
	// The pull behavior is dictated by the pullPolicy, which can have the following behavior
	//   - PullNever: try to use the daemon to return a `local.Image`.
	//   - PullIfNotPResent: try look to use the daemon to return a `local.Image`, if none is found  fetch a remote image.
	//   - PullAlways: it will only try to fetch a remote image.
	//
	// These PullPolicies that these interact with the daemon argument.
	// PullIfNotPresent and daemon = false, gives us the same behavior as PullAlways.
	// There is a single invalid configuration, PullNever and daemon = false, this will always fail.
	Fetch(ctx context.Context, name string, options image.FetchOptions) (imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination ../testmocks/mock_blob_downloader.go github.com/buildpacks/pack/pkg/client BlobDownloader

// BlobDownloader is an interface for collecting both remote and local assets as blobs.
type BlobDownloader interface {
	// Download collects both local and remote assets and provides a blob object
	// used to read asset contents.
	Download(ctx context.Context, pathOrURI string) (blob.Blob, error)
}

//go:generate mockgen -package testmocks -destination ../testmocks/mock_image_factory.go github.com/buildpacks/pack/pkg/client ImageFactory

// ImageFactory is an interface representing the ability to create a new OCI image.
type ImageFactory interface {
	// NewImage initializes an image object with required settings so that it
	// can be written either locally or to a registry.
	NewImage(repoName string, local bool, imageOS string) (imgutil.Image, error)
}

// IndexFactory is an interface representing the ability to create a ImageIndex/ManifestList.
type IndexFactory interface {
	NewIndex(reponame string, opts imgutil.IndexOptions) (imgutil.Index, error)
}

type Runtime interface {
	// LookupManifestList looks up a manifest list with the specified name in the
	// containers storage.
	LookupImageIndex(name string) (index runtime.ImageIndex, err error)
	// LoadFromImage reads the manifest list or image index, and additional
	// information about where the various instances that it contains live, from an
	// image record with the specified ID in local storage.
	LoadFromImage(name string) (imageID string, index imgutil.Index, err error)
	// ExpandNames takes unqualified names, parses them as image names, and returns
	// the fully expanded result, including a tag.  Names which don't include a registry
	// name will be marked for the most-preferred registry
	ExpandIndexNames(names []string) (images []string, err error)
	// ImageType returns the MediaType of the given image's format
	ImageType(format string) (manifestType imgutil.MediaTypes)
	// FindImage locates the locally-stored image which corresponds to a given name.
	FindImage(name string) (name.Reference, imgutil.Image, error)
	// parse name reference
	ParseReference(image string) (name.Reference, error)
	// parses the digest reference
	ParseDigest(image string) (name.Digest, error)
	// RemoveManifests will delete manifest/manifestList from the local stroage
	RemoveManifests(ctx context.Context, names []string) (reports runtime.RemoveImageReport, errors []error)
	// Fetch ManifestList from Registry with the given name
	FetchIndex(name string) (imgutil.Index, error)
}

//go:generate mockgen -package testmocks -destination ../testmocks/mock_buildpack_downloader.go github.com/buildpacks/pack/pkg/client BuildpackDownloader

// BuildpackDownloader is an interface for downloading and extracting buildpacks from various sources
type BuildpackDownloader interface {
	// Download parses a buildpack URI and downloads the buildpack and any dependencies buildpacks from the appropriate source
	Download(ctx context.Context, buildpackURI string, opts buildpack.DownloadOptions) (buildpack.BuildModule, []buildpack.BuildModule, error)
}

// Client is an orchestration object, it contains all parameters needed to
// build an app image using Cloud Native Buildpacks.
// All settings on this object should be changed through ClientOption functions.
type Client struct {
	logger logging.Logger
	docker DockerClient

	keychain            authn.Keychain
	imageFactory        ImageFactory
	indexFactory		IndexFactory
	runtime				Runtime
	imageFetcher        ImageFetcher
	downloader          BlobDownloader
	lifecycleExecutor   LifecycleExecutor
	buildpackDownloader BuildpackDownloader

	experimental    bool
	registryMirrors map[string]string
	version         string
}

// Option is a type of function that mutate settings on the client.
// Values in these functions are set through currying.
type Option func(c *Client)

// WithLogger supply your own logger.
func WithLogger(l logging.Logger) Option {
	return func(c *Client) {
		c.logger = l
	}
}

// WithImageFactory supply your own image factory.
func WithImageFactory(f ImageFactory) Option {
	return func(c *Client) {
		c.imageFactory = f
	}
}

// WithFetcher supply your own Fetcher.
// A Fetcher retrieves both local and remote images to make them available.
func WithFetcher(f ImageFetcher) Option {
	return func(c *Client) {
		c.imageFetcher = f
	}
}

// WithDownloader supply your own downloader.
// A Downloader is used to gather buildpacks from both remote urls, or local sources.
func WithDownloader(d BlobDownloader) Option {
	return func(c *Client) {
		c.downloader = d
	}
}

// WithBuildpackDownloader supply your own BuildpackDownloader.
// A BuildpackDownloader is used to gather buildpacks from both remote urls, or local sources.
func WithBuildpackDownloader(d BuildpackDownloader) Option {
	return func(c *Client) {
		c.buildpackDownloader = d
	}
}

// Deprecated: use WithDownloader instead.
//
// WithCacheDir supply your own cache directory.
func WithCacheDir(path string) Option {
	return func(c *Client) {
		c.downloader = blob.NewDownloader(c.logger, path)
	}
}

// WithDockerClient supply your own docker client.
func WithDockerClient(docker DockerClient) Option {
	return func(c *Client) {
		c.docker = docker
	}
}

// WithExperimental sets whether experimental features should be enabled.
func WithExperimental(experimental bool) Option {
	return func(c *Client) {
		c.experimental = experimental
	}
}

// WithRegistryMirrors sets mirrors to pull images from.
func WithRegistryMirrors(registryMirrors map[string]string) Option {
	return func(c *Client) {
		c.registryMirrors = registryMirrors
	}
}

// WithKeychain sets keychain of credentials to image registries
func WithKeychain(keychain authn.Keychain) Option {
	return func(c *Client) {
		c.keychain = keychain
	}
}

const DockerAPIVersion = "1.38"

// NewClient allocates and returns a Client configured with the specified options.
func NewClient(opts ...Option) (*Client, error) {
	client := &Client{
		version:  pack.Version,
		keychain: authn.DefaultKeychain,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.logger == nil {
		client.logger = logging.NewSimpleLogger(os.Stderr)
	}

	if client.docker == nil {
		var err error
		client.docker, err = dockerClient.NewClientWithOpts(
			dockerClient.FromEnv,
			dockerClient.WithVersion(DockerAPIVersion),
		)
		if err != nil {
			return nil, errors.Wrap(err, "creating docker client")
		}
	}

	if client.downloader == nil {
		packHome, err := iconfig.PackHome()
		if err != nil {
			return nil, errors.Wrap(err, "getting pack home")
		}
		client.downloader = blob.NewDownloader(client.logger, filepath.Join(packHome, "download-cache"))
	}

	if client.imageFetcher == nil {
		client.imageFetcher = image.NewFetcher(client.logger, client.docker, image.WithRegistryMirrors(client.registryMirrors), image.WithKeychain(client.keychain))
	}

	if client.runtime == nil {
		client.runtime = runtime.NewRuntime()
	}

	if client.imageFactory == nil {
		client.imageFactory = &imageFactory{
			dockerClient: client.docker,
			keychain:     client.keychain,
		}
	}

	if client.indexFactory == nil {
		client.indexFactory = &indexFactory{
			keychain: client.keychain,
		}
	}

	if client.buildpackDownloader == nil {
		client.buildpackDownloader = buildpack.NewDownloader(
			client.logger,
			client.imageFetcher,
			client.downloader,
			&registryResolver{
				logger: client.logger,
			},
		)
	}

	client.lifecycleExecutor = build.NewLifecycleExecutor(client.logger, client.docker)

	return client, nil
}

type registryResolver struct {
	logger logging.Logger
}

func (r *registryResolver) Resolve(registryName, bpName string) (string, error) {
	cache, err := getRegistry(r.logger, registryName)
	if err != nil {
		return "", errors.Wrapf(err, "lookup registry %s", style.Symbol(registryName))
	}

	regBuildpack, err := cache.LocateBuildpack(bpName)
	if err != nil {
		return "", errors.Wrapf(err, "lookup buildpack %s", style.Symbol(bpName))
	}

	return regBuildpack.Address, nil
}

type imageFactory struct {
	dockerClient local.DockerClient
	keychain     authn.Keychain
}

func (f *imageFactory) NewImage(repoName string, daemon bool, imageOS string) (imgutil.Image, error) {
	platform := imgutil.Platform{OS: imageOS}

	if daemon {
		return local.NewImage(repoName, f.dockerClient, local.WithDefaultPlatform(platform))
	}

	return remote.NewImage(repoName, f.keychain, remote.WithDefaultPlatform(platform))
}

type indexFactory struct {
	keychain authn.Keychain
}

func(f *indexFactory) NewIndex(name string, opts imgutil.IndexOptions) (index imgutil.Index, err error) {

	index, err = remote.NewIndex(name, f.keychain, opts)
	if err != nil {
		if opts.MediaType == imgutil.MediaTypes.OCI {
			return layout.NewIndex(name, f.keychain, opts)
		} else {
			return local.NewIndex(name, f.keychain, opts)
		}
	}

	return index, err
}
