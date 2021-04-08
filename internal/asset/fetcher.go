package asset

import (
	"context"
	"fmt"
	"github.com/buildpacks/imgutil"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/ocipackage"
	"os"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_image_fetcher.go github.com/buildpacks/pack/internal/asset ImageFetcher
type ImageFetcher interface {
	FetchImageAssets(ctx context.Context, pullPolicy pubcfg.PullPolicy, imageNames...string) ([]imgutil.Image, error)
}

//go:generate mockgen -package testmocks -destination testmocks/mock_uri_fetcher.go github.com/buildpacks/pack/internal/asset URIFetcher
type URIFetcher interface {
	FetchURIAssets(ctx context.Context, fileAssets ...string) ([]*ocipackage.OciLayoutPackage, error)
}

type Fetcher struct {
	assetFileFetcher  FileFetcher
	assetURIFetcher   URIFetcher
	assetImageFetcher ImageFetcher
}

func NewFetcher(assetFileFetcher FileFetcher, assetURIFetcher URIFetcher, assetImageFetcher ImageFetcher) Fetcher {
	return Fetcher{
		assetFileFetcher:  assetFileFetcher,
		assetURIFetcher:   assetURIFetcher,
		assetImageFetcher: assetImageFetcher,
	}
}

type AssetFetcherConfig struct {
	ctx             context.Context
	imagePullPolicy pubcfg.PullPolicy
	workingDir    string
}

func DefaultAssetFetcherConfig() (AssetFetcherConfig, error) {
	wd, err := os.Getwd()
	if err != nil {
		return AssetFetcherConfig{}, fmt.Errorf("unable to create asset fetcher config: %q", err)
	}
	return AssetFetcherConfig{
		ctx:             context.Background(),
		imagePullPolicy: pubcfg.PullIfNotPresent,
		workingDir:    wd,
	}, nil
}

type AssetFetcherOptions func(*AssetFetcherConfig)

func WithPullPolicy(policy pubcfg.PullPolicy) AssetFetcherOptions {
	return func(cfg *AssetFetcherConfig) {
		cfg.imagePullPolicy = policy
	}
}

func WithContext(ctx context.Context) AssetFetcherOptions {
	return func (cfg *AssetFetcherConfig) {
		cfg.ctx = ctx
	}
}

func WithWorkingDir(workingDir string) AssetFetcherOptions {
	return func (cfg *AssetFetcherConfig) {
		cfg.workingDir = workingDir
	}
}

func (a Fetcher) FetchAssets(assetNameList []string, options ...AssetFetcherOptions) ([]Readable, error) {
	result := []Readable{}

	cfg, err := DefaultAssetFetcherConfig()
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

func castOCIToReadable(OCIAssets []*ocipackage.OciLayoutPackage) []Readable {
	result := []Readable{}
	for _, pkg := range OCIAssets {
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
