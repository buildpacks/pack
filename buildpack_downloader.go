package pack

import (
	"context"
	"fmt"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/buildpack"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	"github.com/pkg/errors"
)

type buildpackDownloader struct {
	logger       logging.Logger
	imageFetcher ImageFetcher
	downloader   Downloader
}

func NewBuildpackDownloader(logger logging.Logger, imageFetcher ImageFetcher, downloader Downloader) *buildpackDownloader { //nolint:golint,gosimple
	return &buildpackDownloader{
		logger:       logger,
		imageFetcher: imageFetcher,
		downloader:   downloader,
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
		mainBP, depBPs, err = extractPackagedBuildpacks(ctx, imageName, c.imageFetcher, opts.Daemon, opts.PullPolicy)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "extracting from registry %s", style.Symbol(buildpackURI))
		}
	case buildpack.RegistryLocator:
		c.logger.Debugf("Downloading buildpack from registry: %s", style.Symbol(buildpackURI))
		registryCache, err := getRegistry(c.logger, opts.RegistryName)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "invalid registry '%s'", opts.RegistryName)
		}

		registryBp, err := registryCache.LocateBuildpack(buildpackURI)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "locating in registry %s", style.Symbol(buildpackURI))
		}

		mainBP, depBPs, err = extractPackagedBuildpacks(ctx, registryBp.Address, c.imageFetcher, opts.Daemon, opts.PullPolicy)
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
