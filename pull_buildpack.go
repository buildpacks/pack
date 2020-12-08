package pack

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/buildpack"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

// PullBuildpackOptions are options available for PullBuildpack
type PullBuildpackOptions struct {
	// URI of the buildpack to retrieve.
	URI string
	// RegistryName to search for buildpacks from.
	RegistryName string
	// RelativeBaseDir to resolve relative assests from.
	RelativeBaseDir string
}

// PullBuildpack pulls given buildpack to be stored locally
func (c *Client) PullBuildpack(ctx context.Context, opts PullBuildpackOptions) error {
	locatorType, err := buildpack.GetLocatorType(opts.URI, "", []dist.BuildpackInfo{})
	if err != nil {
		return err
	}

	switch locatorType {
	case buildpack.PackageLocator:
		imageName := buildpack.ParsePackageLocator(opts.URI)
		c.logger.Debugf("Pulling buildpack from image: %s", imageName)

		_, err = c.imageFetcher.Fetch(ctx, imageName, true, config.PullAlways)
		if err != nil {
			return errors.Wrapf(err, "fetching image %s", style.Symbol(opts.URI))
		}
	case buildpack.RegistryLocator:
		c.logger.Debugf("Pulling buildpack from registry: %s", style.Symbol(opts.URI))
		registryCache, err := c.getRegistry(c.logger, opts.RegistryName)

		if err != nil {
			return errors.Wrapf(err, "invalid registry '%s'", opts.RegistryName)
		}

		registryBp, err := registryCache.LocateBuildpack(opts.URI)
		if err != nil {
			return errors.Wrapf(err, "locating in registry %s", style.Symbol(opts.URI))
		}

		_, err = c.imageFetcher.Fetch(ctx, registryBp.Address, true, config.PullAlways)
		if err != nil {
			return errors.Wrapf(err, "fetching image %s", style.Symbol(opts.URI))
		}
	case buildpack.InvalidLocator:
		return fmt.Errorf("invalid buildpack URI %s", style.Symbol(opts.URI))
	default:
		return fmt.Errorf("unsupported buildpack URI type: %s", style.Symbol(locatorType.String()))
	}

	return nil
}
