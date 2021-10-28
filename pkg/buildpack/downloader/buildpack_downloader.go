package downloader

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/pack/internal/buildpack"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/config"
	"github.com/buildpacks/pack/pkg/image"
)

type ImageFetcher interface {
	Fetch(ctx context.Context, name string, options image.FetchOptions) (imgutil.Image, error)
}

type Downloader interface {
	Download(ctx context.Context, pathOrURI string) (blob.Blob, error)
}

//go:generate mockgen -package testmocks -destination ../../testmocks/mock_registry_resolver.go github.com/buildpacks/pack/pkg/buildpack/downloader RegistryResolver

type RegistryResolver interface {
	Resolve(registryName, bpURI string) (string, error)
}

type buildpackDownloader struct {
	logger           logging.Logger
	imageFetcher     ImageFetcher
	downloader       Downloader
	registryResolver RegistryResolver
}

func NewBuildpackDownloader(logger logging.Logger, imageFetcher ImageFetcher, downloader Downloader, registryResolver RegistryResolver) *buildpackDownloader { //nolint:golint,gosimple
	return &buildpackDownloader{
		logger:           logger,
		imageFetcher:     imageFetcher,
		downloader:       downloader,
		registryResolver: registryResolver,
	}
}

type BuildpackDownloadOptions struct {
	// Buildpack registry name. Defines where all registry buildpacks will be pulled from.
	RegistryName string

	// The base directory to use to resolve relative assets
	RelativeBaseDir string

	// The OS of the builder image
	ImageOS string

	// Deprecated, the older alternative to buildpack URI
	ImageName string

	Daemon bool

	PullPolicy config.PullPolicy
}

func (c *buildpackDownloader) Download(ctx context.Context, buildpackURI string, opts BuildpackDownloadOptions) (dist.Buildpack, []dist.Buildpack, error) {
	var err error
	var locatorType buildpack.LocatorType
	if buildpackURI == "" && opts.ImageName != "" {
		c.logger.Warn("The 'image' key is deprecated. Use 'uri=\"docker://...\"' instead.")
		buildpackURI = opts.ImageName
		locatorType = buildpack.PackageLocator
	} else {
		locatorType, err = buildpack.GetLocatorType(buildpackURI, opts.RelativeBaseDir, []dist.BuildpackInfo{})
		if err != nil {
			return nil, nil, err
		}
	}

	var mainBP dist.Buildpack
	var depBPs []dist.Buildpack
	switch locatorType {
	case buildpack.PackageLocator:
		imageName := buildpack.ParsePackageLocator(buildpackURI)
		c.logger.Debugf("Downloading buildpack from image: %s", style.Symbol(imageName))
		mainBP, depBPs, err = extractPackagedBuildpacks(ctx, imageName, c.imageFetcher, image.FetchOptions{Daemon: opts.Daemon, PullPolicy: opts.PullPolicy})
		if err != nil {
			return nil, nil, errors.Wrapf(err, "extracting from registry %s", style.Symbol(buildpackURI))
		}
	case buildpack.RegistryLocator:
		c.logger.Debugf("Downloading buildpack from registry: %s", style.Symbol(buildpackURI))
		address, err := c.registryResolver.Resolve(opts.RegistryName, buildpackURI)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "locating in registry: %s", style.Symbol(buildpackURI))
		}

		mainBP, depBPs, err = extractPackagedBuildpacks(ctx, address, c.imageFetcher, image.FetchOptions{Daemon: opts.Daemon, PullPolicy: opts.PullPolicy})
		if err != nil {
			return nil, nil, errors.Wrapf(err, "extracting from registry %s", style.Symbol(buildpackURI))
		}
	case buildpack.URILocator:
		buildpackURI, err = paths.FilePathToURI(buildpackURI, opts.RelativeBaseDir)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "making absolute: %s", style.Symbol(buildpackURI))
		}

		c.logger.Debugf("Downloading buildpack from URI: %s", style.Symbol(buildpackURI))

		blob, err := c.downloader.Download(ctx, buildpackURI)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(buildpackURI))
		}

		mainBP, depBPs, err = decomposeBuildpack(blob, opts.ImageOS)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "extracting from %s", style.Symbol(buildpackURI))
		}
	default:
		return nil, nil, fmt.Errorf("error reading %s: invalid locator: %s", buildpackURI, locatorType)
	}
	return mainBP, depBPs, nil
}

// decomposeBuildpack decomposes a buildpack blob into the main builder (order buildpack) and it's dependencies buildpacks.
func decomposeBuildpack(blob blob.Blob, imageOS string) (mainBP dist.Buildpack, depBPs []dist.Buildpack, err error) {
	isOCILayout, err := buildpackage.IsOCILayoutBlob(blob)
	if err != nil {
		return mainBP, depBPs, errors.Wrap(err, "inspecting buildpack blob")
	}

	if isOCILayout {
		mainBP, depBPs, err = buildpackage.BuildpacksFromOCILayoutBlob(blob)
		if err != nil {
			return mainBP, depBPs, errors.Wrap(err, "extracting buildpacks")
		}
	} else {
		layerWriterFactory, err := layer.NewWriterFactory(imageOS)
		if err != nil {
			return mainBP, depBPs, errors.Wrapf(err, "get tar writer factory for OS %s", style.Symbol(imageOS))
		}

		mainBP, err = dist.BuildpackFromRootBlob(blob, layerWriterFactory)
		if err != nil {
			return mainBP, depBPs, errors.Wrap(err, "reading buildpack")
		}
	}

	return mainBP, depBPs, nil
}

func extractPackagedBuildpacks(ctx context.Context, pkgImageRef string, fetcher ImageFetcher, fetchOptions image.FetchOptions) (mainBP dist.Buildpack, depBPs []dist.Buildpack, err error) {
	pkgImage, err := fetcher.Fetch(ctx, pkgImageRef, fetchOptions)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "fetching image")
	}

	mainBP, depBPs, err = buildpackage.ExtractBuildpacks(pkgImage)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "extracting buildpacks from %s", style.Symbol(pkgImageRef))
	}

	return mainBP, depBPs, nil
}
