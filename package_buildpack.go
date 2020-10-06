package pack

import (
	"context"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/layer"

	"github.com/pkg/errors"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

const (
	// Packaging indicator that format of inputs/outputs will be an OCI image on the registry.
	FormatImage = "image"

	// Packaging indicator that format of output will be a file on the host filesystem.
	FormatFile = "file"
)

// PackageBuildpackOptions is a configuration object used to define
// the behavior of PackageBuildpack.
type PackageBuildpackOptions struct {
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
}

// PackageBuildpack packages buildpack(s) into either an image or file.
func (c *Client) PackageBuildpack(ctx context.Context, opts PackageBuildpackOptions) error {
	info, err := c.docker.Info(ctx)
	if err != nil {
		return errors.Wrap(err, "getting docker info")
	}

	writerFactory, err := layer.NewWriterFactory(info.OSType)
	if err != nil {
		return errors.Wrap(err, "creating layer writer factory")
	}

	packageBuilder := buildpackage.NewBuilder(c.imageFactory)

	if opts.Format == "" {
		opts.Format = FormatImage
	}

	bpURI := opts.Config.Buildpack.URI
	if bpURI == "" {
		return errors.New("buildpack URI must be provided")
	}

	blob, err := c.downloader.Download(ctx, bpURI)
	if err != nil {
		return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(bpURI))
	}

	bp, err := dist.BuildpackFromRootBlob(blob, writerFactory)
	if err != nil {
		return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bpURI))
	}

	packageBuilder.SetBuildpack(bp)

	for _, dep := range opts.Config.Dependencies {
		var depBPs []dist.Buildpack

		if dep.URI != "" {
			blob, err := c.downloader.Download(ctx, dep.URI)
			if err != nil {
				return errors.Wrapf(err, "downloading buildpack from %s", style.Symbol(dep.URI))
			}

			isOCILayout, err := buildpackage.IsOCILayoutBlob(blob)
			if err != nil {
				return errors.Wrap(err, "inspecting buildpack blob")
			}

			if isOCILayout {
				mainBP, deps, err := buildpackage.BuildpacksFromOCILayoutBlob(blob)
				if err != nil {
					return errors.Wrapf(err, "extracting buildpacks from %s", style.Symbol(dep.URI))
				}

				depBPs = append([]dist.Buildpack{mainBP}, deps...)
			} else {
				depBP, err := dist.BuildpackFromRootBlob(blob, writerFactory)
				if err != nil {
					return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(dep.URI))
				}
				depBPs = []dist.Buildpack{depBP}
			}
		} else if dep.ImageName != "" {
			mainBP, deps, err := extractPackagedBuildpacks(ctx, dep.ImageName, c.imageFetcher, opts.Publish, opts.PullPolicy)
			if err != nil {
				return err
			}

			depBPs = append([]dist.Buildpack{mainBP}, deps...)
		}

		for _, depBP := range depBPs {
			packageBuilder.AddDependency(depBP)
		}
	}

	switch opts.Format {
	case FormatFile:
		return packageBuilder.SaveAsFile(opts.Name, info.OSType)
	case FormatImage:
		_, err = packageBuilder.SaveAsImage(opts.Name, opts.Publish, info.OSType)
		return errors.Wrapf(err, "saving image")
	default:
		return errors.Errorf("unknown format: %s", style.Symbol(opts.Format))
	}
}
