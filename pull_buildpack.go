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
	case buildpack.PackageLocator:
		imageName := buildpack.ParsePackageLocator(opts.URI)
		_, err = c.imageFetcher.Fetch(ctx, imageName, true, config.PullAlways)
		if err != nil {
			return errors.Wrapf(err, "fetching image %s", style.Symbol(opts.URI))
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

		_, err = c.imageFetcher.Fetch(ctx, registryBp.Address, true, config.PullAlways)
		if err != nil {
			return errors.Wrapf(err, "fetching image %s", style.Symbol(opts.URI))
		}
	default:
		return fmt.Errorf("invalid buildpack URI %s", style.Symbol(opts.URI))
	}

	return nil
}
