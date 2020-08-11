package pack

import (
	"context"

	"github.com/pkg/errors"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/archive"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

const (
	// Indicator that format of inputs/outputs will be an OCI image on the registry
	FormatImage = "image"

	// Indicator that format of inputs/outputs will be a file on the host filesystem
	FormatFile  = "file"
)

// PackageBuildpackOptions are configuration options and metadata for PackageBuildpack
type PackageBuildpackOptions struct {
	// the name of the output artifact
	Name    string

	// Type of output format, the options are the consts FormatImage, and FormatFile
	Format  string

	// Buildpack configuration
	Config  pubbldpkg.Config

	// Push resulting builder image up to registry specified in Name
	Publish bool

	//Use only local image assets.
	NoPull  bool
}

// PackageBuildpack packages buildpack(s) into an image or file
func (c *Client) PackageBuildpack(ctx context.Context, opts PackageBuildpackOptions) error {
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

	bp, err := dist.BuildpackFromRootBlob(blob, archive.DefaultTarWriterFactory())
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
				depBP, err := dist.BuildpackFromRootBlob(blob, archive.DefaultTarWriterFactory())
				if err != nil {
					return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(dep.URI))
				}
				depBPs = []dist.Buildpack{depBP}
			}
		} else if dep.ImageName != "" {
			mainBP, deps, err := extractPackagedBuildpacks(ctx, dep.ImageName, c.imageFetcher, opts.Publish, opts.NoPull)
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
		return packageBuilder.SaveAsFile(opts.Name)
	case FormatImage:
		_, err = packageBuilder.SaveAsImage(opts.Name, opts.Publish)
		return errors.Wrapf(err, "saving image")
	default:
		return errors.Errorf("unknown format: %s", style.Symbol(opts.Format))
	}
}
