package asset

import (
	"context"
	"fmt"
	"os"

	"github.com/buildpacks/imgutil"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/ocipackage"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_image_fetcher.go github.com/buildpacks/pack/internal/asset ImageCacheFetcher
type ImageCacheFetcher interface {
	FetchImageAssets(ctx context.Context, pullPolicy pubcfg.PullPolicy, imageNames ...string) ([]imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_uri_fetcher.go github.com/buildpacks/pack/internal/asset URICacheFetcher
type URICacheFetcher interface {
	FetchURIAssets(ctx context.Context, fileAssets ...string) ([]*ocipackage.OciLayoutPackage, error)
}

type Fetcher struct {
	assetFileFetcher  FileCacheFetcher
	assetURIFetcher   URICacheFetcher
	assetImageFetcher ImageCacheFetcher
}

func NewFetcher(assetFileFetcher FileCacheFetcher, assetURIFetcher URICacheFetcher, assetImageFetcher ImageCacheFetcher) Fetcher {
	return Fetcher{
		assetFileFetcher:  assetFileFetcher,
		assetURIFetcher:   assetURIFetcher,
		assetImageFetcher: assetImageFetcher,
	}
}

type FetcherConfig struct {
	ctx             context.Context
	imagePullPolicy pubcfg.PullPolicy
	workingDir      string
}

func DefaultFetcherConfig() (FetcherConfig, error) {
	wd, err := os.Getwd()
	if err != nil {
		return FetcherConfig{}, fmt.Errorf("unable to create asset fetcher config: %q", err)
	}
	return FetcherConfig{
		ctx:             context.Background(),
		imagePullPolicy: pubcfg.PullIfNotPresent,
		workingDir:      wd,
	}, nil
}

type FetcherOptions func(*FetcherConfig)

func WithPullPolicy(policy pubcfg.PullPolicy) FetcherOptions {
	return func(cfg *FetcherConfig) {
		cfg.imagePullPolicy = policy
	}
}

func WithContext(ctx context.Context) FetcherOptions {
	return func(cfg *FetcherConfig) {
		cfg.ctx = ctx
	}
}

func WithWorkingDir(workingDir string) FetcherOptions {
	return func(cfg *FetcherConfig) {
		cfg.workingDir = workingDir
	}
}

func (a Fetcher) FetchAssets(assetNameList []string, options ...FetcherOptions) ([]Readable, error) {
	result := []Readable{}

	cfg, err := DefaultFetcherConfig()
	if err != nil {
		return []Readable{}, err
	}
	for _, option := range options {
		option(&cfg)
	}

	for _, assetName := range assetNameList {
		locator := GetLocatorType(assetName, cfg.workingDir)
		var assets []Readable
		var OCIAssets []*ocipackage.OciLayoutPackage
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

func castOCIToReadable(ociAssets []*ocipackage.OciLayoutPackage) []Readable {
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
