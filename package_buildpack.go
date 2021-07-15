package pack

import (
	"context"

	"github.com/pkg/errors"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/blob"
	"github.com/buildpacks/pack/internal/buildpack"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
)

const (
	// Packaging indicator that format of inputs/outputs will be an OCI image on the registry.
	FormatImage = "image"

	// Packaging indicator that format of output will be a file on the host filesystem.
	FormatFile = "file"

	// CNBExtension is the file extension for a cloud native buildpack tar archive
	CNBExtension = ".cnb"
)

// PackageBuildpackOptions is a configuration object used to define
// the behavior of PackageBuildpack.
type PackageBuildpackOptions struct {
	// The base director to resolve relative assest from
	RelativeBaseDir string

	// The name of the output buildpack artifact.
	Name string

	// Type of output format, The options are the either the const FormatImage, or FormatFile.
	Format string

	// Defines the Buildpacks configuration.
	Config pubbldpkg.Config

	// Push resulting builder image up to a registry
	// specified in the Name variable.
	Publish bool

	// Strategy for updating images before packaging.
	PullPolicy config.PullPolicy

	// Name of the buildpack registry. Used to
	// add buildpacks to a package.
	Registry string
}

// PackageBuildpack packages buildpack(s) into either an image or file.
func (c *Client) PackageBuildpack(ctx context.Context, opts PackageBuildpackOptions) error {
	if opts.Format == "" {
		opts.Format = FormatImage
	}

	if opts.Config.Platform.OS == "windows" && !c.experimental {
		return NewExperimentError("Windows buildpackage support is currently experimental.")
	}

	err := c.validateOSPlatform(ctx, opts.Config.Platform.OS, opts.Publish, opts.Format)
	if err != nil {
		return err
	}

	writerFactory, err := layer.NewWriterFactory(opts.Config.Platform.OS)
	if err != nil {
		return errors.Wrap(err, "creating layer writer factory")
	}

	packageBuilder := buildpackage.NewBuilder(c.imageFactory)

	bpURI := opts.Config.Buildpack.URI
	if bpURI == "" {
		return errors.New("buildpack URI must be provided")
	}

	mainBlob, err := c.downloadBuildpackFromURI(ctx, bpURI, opts.RelativeBaseDir)
	if err != nil {
		return err
	}

	bp, err := dist.BuildpackFromRootBlob(mainBlob, writerFactory)
	if err != nil {
		return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bpURI))
	}

	packageBuilder.SetBuildpack(bp)

	for _, dep := range opts.Config.Dependencies {
		var depBPs []dist.Buildpack

		if dep.ImageName != "" {
			c.logger.Warn("The 'image' key is deprecated. Use 'uri=\"docker://...\"' instead.")
			mainBP, deps, err := extractPackagedBuildpacks(ctx, dep.ImageName, c.imageFetcher, image.FetchOptions{Daemon: !opts.Publish, PullPolicy: opts.PullPolicy})
			if err != nil {
				return err
			}

			depBPs = append([]dist.Buildpack{mainBP}, deps...)
		} else if dep.URI != "" {
			locatorType, err := buildpack.GetLocatorType(dep.URI, opts.RelativeBaseDir, nil)
			if err != nil {
				return err
			}

			switch locatorType {
			case buildpack.URILocator:
				depBlob, err := c.downloadBuildpackFromURI(ctx, dep.URI, opts.RelativeBaseDir)
				if err != nil {
					return err
				}

				isOCILayout, err := buildpackage.IsOCILayoutBlob(depBlob)
				if err != nil {
					return errors.Wrap(err, "inspecting buildpack blob")
				}

				if isOCILayout {
					mainBP, deps, err := buildpackage.BuildpacksFromOCILayoutBlob(depBlob)
					if err != nil {
						return errors.Wrapf(err, "extracting buildpacks from %s", style.Symbol(dep.URI))
					}

					depBPs = append([]dist.Buildpack{mainBP}, deps...)
				} else {
					depBP, err := dist.BuildpackFromRootBlob(depBlob, writerFactory)
					if err != nil {
						return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(dep.URI))
					}
					depBPs = []dist.Buildpack{depBP}
				}
			case buildpack.PackageLocator:
				imageName := buildpack.ParsePackageLocator(dep.URI)
				c.logger.Debugf("Downloading buildpack from image: %s", style.Symbol(imageName))
				mainBP, deps, err := extractPackagedBuildpacks(ctx, imageName, c.imageFetcher, image.FetchOptions{Daemon: !opts.Publish, PullPolicy: opts.PullPolicy})
				if err != nil {
					return err
				}

				depBPs = append([]dist.Buildpack{mainBP}, deps...)
			case buildpack.RegistryLocator:
				registryCache, err := getRegistry(c.logger, opts.Registry)
				if err != nil {
					return errors.Wrapf(err, "invalid registry '%s'", opts.Registry)
				}

				registryBp, err := registryCache.LocateBuildpack(dep.URI)
				if err != nil {
					return errors.Wrapf(err, "locating in registry %s", style.Symbol(dep.URI))
				}

				mainBP, deps, err := extractPackagedBuildpacks(ctx, registryBp.Address, c.imageFetcher, image.FetchOptions{Daemon: !opts.Publish, PullPolicy: opts.PullPolicy})
				if err != nil {
					return errors.Wrapf(err, "extracting from registry %s", style.Symbol(dep.URI))
				}

				depBPs = append([]dist.Buildpack{mainBP}, deps...)
			case buildpack.InvalidLocator:
				return errors.Errorf("invalid locator %s", style.Symbol(dep.URI))
			default:
				return errors.Errorf("unsupported locator type %s", style.Symbol(locatorType.String()))
			}
		}

		for _, depBP := range depBPs {
			packageBuilder.AddDependency(depBP)
		}
	}

	switch opts.Format {
	case FormatFile:
		return packageBuilder.SaveAsFile(opts.Name, opts.Config.Platform.OS)
	case FormatImage:
		_, err = packageBuilder.SaveAsImage(opts.Name, opts.Publish, opts.Config.Platform.OS)
		return errors.Wrapf(err, "saving image")
	default:
		return errors.Errorf("unknown format: %s", style.Symbol(opts.Format))
	}
}

func (c *Client) downloadBuildpackFromURI(ctx context.Context, uri, relativeBaseDir string) (blob.Blob, error) {
	absPath, err := paths.FilePathToURI(uri, relativeBaseDir)
	if err != nil {
		return nil, errors.Wrapf(err, "making absolute: %s", style.Symbol(uri))
	}
	uri = absPath

	c.logger.Debugf("Downloading buildpack from URI: %s", style.Symbol(uri))
	blob, err := c.downloader.Download(ctx, uri)
	if err != nil {
		return nil, errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(uri))
	}

	return blob, nil
}

func (c *Client) validateOSPlatform(ctx context.Context, os string, publish bool, format string) error {
	if publish || format == FormatFile {
		return nil
	}

	info, err := c.docker.Info(ctx)
	if err != nil {
		return err
	}

	if info.OSType != os {
		return errors.Errorf("invalid %s specified: DOCKER_OS is %s", style.Symbol("platform.os"), style.Symbol(info.OSType))
	}

	return nil
}
