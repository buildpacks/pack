package asset

import (
	"context"
	"fmt"
	"os"

	"github.com/buildpacks/imgutil"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/oci"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_image_fetcher.go github.com/buildpacks/pack/internal/asset ImageFetcher
// ImageFetcher is implemented by Fetcher which allows work with remote and local images, 
// as well as control when images are used locally vs pulled remotely.
type ImageFetcher interface {
	
	// FetchImageAssets returns a list of images that were retrieved either from the local 
	// Docker Daemon, or a remote registry. Preference about when images are pulled can be controlled using
	// pullPolicy.
	FetchImageAssets(ctx context.Context, pullPolicy pubcfg.PullPolicy, imageNames ...string) ([]imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_uri_fetcher.go github.com/buildpacks/pack/internal/asset URIFetcher

// URIFetcher is implemented by URIPackageFetcher
type URIFetcher interface {
	FetchURIAssets(ctx context.Context, fileAssets ...string) ([]*oci.LayoutPackage, error)
}

// Fetcher holds internal state needed to retrieve OCI Image blobs from the following locations
// - local files
// - local or remote images
// - http/https urls.
type Fetcher struct {
	assetFileFetcher  FileFetcher
	assetURIFetcher   URIFetcher
	assetImageFetcher ImageFetcher
}

// NewFetcher is a constructor used to create new Fetcher objects.
func NewFetcher(assetFileFetcher FileFetcher, assetURIFetcher URIFetcher, assetImageFetcher ImageFetcher) Fetcher {
	return Fetcher{
		assetFileFetcher:  assetFileFetcher,
		assetURIFetcher:   assetURIFetcher,
		assetImageFetcher: assetImageFetcher,
	}
}

type fetcherConfig struct {
	ctx             context.Context
	imagePullPolicy pubcfg.PullPolicy
	workingDir      string
}

func defaultFetcherConfig() (fetcherConfig, error) {
	wd, err := os.Getwd()
	if err != nil {
		return fetcherConfig{}, fmt.Errorf("unable to create asset fetcher config: %q", err)
	}
	return fetcherConfig{
		ctx:             context.Background(),
		imagePullPolicy: pubcfg.PullIfNotPresent,
		workingDir:      wd,
	}, nil
}

// FetcherOption represents configuration when calling FetchAssets
type FetcherOptions func(*fetcherConfig)

// WithPullPolicy sets the pull policy used when fetching image assets
func WithPullPolicy(policy pubcfg.PullPolicy) FetcherOptions {
	return func(cfg *fetcherConfig) {
		cfg.imagePullPolicy = policy
	}
}

// WithContext sets the context used for all image & uri based downloads.
func WithContext(ctx context.Context) FetcherOptions {
	return func(cfg *fetcherConfig) {
		cfg.ctx = ctx
	}
}

// WithWorkingDir sets the working dir used to resolve the filepaths of
// local file assets.
func WithWorkingDir(workingDir string) FetcherOptions {
	return func(cfg *fetcherConfig) {
		cfg.workingDir = workingDir
	}
}

// FetchAssets takes a list of names and a variadic list of configuration options
// it will then return a list of Readable objects, each should correspond to an asset image.
func (a Fetcher) FetchAssets(assetNameList []string, options ...FetcherOptions) ([]Readable, error) {
	result := []Readable{}

	cfg, err := defaultFetcherConfig()
	if err != nil {
		return []Readable{}, err
	}
	for _, option := range options {
		option(&cfg)
	}

	for _, assetName := range assetNameList {
		locator := GetLocatorType(assetName, cfg.workingDir)
		var assets []Readable
		var OCIAssets []*oci.LayoutPackage
		var imgAssets []imgutil.Image
		switch locator {
		case URILocator:
			OCIAssets, err = a.assetURIFetcher.FetchURIAssets(cfg.ctx, assetName)
			assets = castOCIToReadable(OCIAssets)
		case FilepathLocator:
			OCIAssets, err = a.assetFileFetcher.FetchFileAssets(cfg.ctx, cfg.workingDir, assetName)
			assets = castOCIToReadable(OCIAssets)
		case ImageLocator:
			imgAssets, err = a.assetImageFetcher.FetchImageAssets(cfg.ctx, cfg.imagePullPolicy, assetName)
			assets = castImgToReadable(imgAssets)
		default:
			return result, fmt.Errorf("unable to determine asset type from name: %s", assetName)
		}
		if err != nil {
			return result, fmt.Errorf("unable to fetch asset of type %q: %s", locator.String(), err)
		}
		result = append(result, assets...)
	}

	return result, nil
}

func castOCIToReadable(ociAssets []*oci.LayoutPackage) []Readable {
	result := []Readable{}
	for _, pkg := range ociAssets {
		result = append(result, Readable(pkg))
	}

	return result
}

func castImgToReadable(imgs []imgutil.Image) []Readable {
	result := []Readable{}
	for _, pkg := range imgs {
		result = append(result, Readable(pkg))
	}

	return result
}
