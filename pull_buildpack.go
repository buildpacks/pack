package pack

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/buildpack"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

type PullBuildpackOptions struct {
	URI          string
	RegistryType string
	RegistryURL  string
	RegistryName string
}

func (c *Client) PullBuildpack(ctx context.Context, opts PullBuildpackOptions) error {
	locatorType, err := buildpack.GetLocatorType(opts.URI, []dist.BuildpackInfo{})
	if err != nil {
		return err
	}

	switch locatorType {
	case buildpack.URILocator:
		err := ensureBPSupport(opts.URI)
		if err != nil {
			return errors.Wrapf(err, "checking support")
		}

		blob, err := c.downloader.Download(ctx, opts.URI)
		if err != nil {
			return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(opts.URI))
		}

		isOCILayout, err := buildpackage.IsOCILayoutBlob(blob)
		if err != nil {
			return errors.Wrapf(err, "checking format")
		}

		if isOCILayout {
			_, _, err = buildpackage.BuildpacksFromOCILayoutBlob(blob)
			if err != nil {
				return errors.Wrapf(err, "extracting buildpacks from %s", style.Symbol(opts.URI))
			}
		} else {
			return errors.Errorf("buildpack is in an unsupported layout")
		}
	case buildpack.PackageLocator:
		imageName := buildpack.ParsePackageLocator(opts.URI)
		_, _, err := extractPackagedBuildpacks(ctx, imageName, c.imageFetcher, false, config.PullAlways)
		if err != nil {
			return errors.Wrapf(err, "creating from buildpackage %s", style.Symbol(opts.URI))
		}
	case buildpack.RegistryLocator:
		registryCache, err := c.getRegistry(c.logger, opts.RegistryName)
		if err != nil {
			return errors.Wrapf(err, "invalid registry '%s'", opts.RegistryName)
		}

		registryBp, err := registryCache.LocateBuildpack(opts.URI)
		if err != nil {
			return errors.Wrapf(err, "locating in registry %s", style.Symbol(opts.URI))
		}

		_, _, err = extractPackagedBuildpacks(ctx, registryBp.Address, c.imageFetcher, false, config.PullAlways)
		if err != nil {
			return errors.Wrapf(err, "extracting from registry %s", style.Symbol(opts.URI))
		}
	default:
		return fmt.Errorf("invalid buildpack URI %s", style.Symbol(opts.URI))
	}

	return nil
}
