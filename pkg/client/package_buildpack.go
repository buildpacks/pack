package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
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
	PullPolicy image.PullPolicy

	// Name of the buildpack registry. Used to
	// add buildpacks to a package.
	Registry string

	// Flatten layers
	Flatten bool

	// List of buildpack images to exclude from the package been flattened.
	FlattenExclude []string

	// Map of labels to add to the Buildpack
	Labels map[string]string

	// Targets platform for each Buildpack package to be built
	Targets []dist.Target
}

// PackageBuildpack packages buildpack(s) into either an image or file.
func (c *Client) PackageBuildpack(ctx context.Context, opts PackageBuildpackOptions) error {
	if opts.Format == "" {
		opts.Format = FormatImage
	}

	var targets []dist.Target
	if len(opts.Targets) > 0 {
		// when exporting to the daemon, we need to select just one target
		if !opts.Publish && opts.Format == FormatImage {
			daemonTarget, err := c.daemonTarget(ctx, opts.Targets)
			if err != nil {
				return err
			}
			targets = append(targets, daemonTarget)
		} else {
			targets = opts.Targets
		}
	} else {
		targets = append(targets, dist.Target{OS: opts.Config.Platform.OS})
	}

	multiArch := len(targets) > 1 && (opts.Publish || opts.Format == FormatFile)

	var digests []string
	for _, target := range targets {
		if target.OS == "windows" && !c.experimental {
			return NewExperimentError("Windows buildpackage support is currently experimental.")
		}

		err := c.validateOSPlatform(ctx, target.OS, opts.Publish, opts.Format)
		if err != nil {
			return err
		}

		writerFactory, err := layer.NewWriterFactory(target.OS)
		if err != nil {
			return errors.Wrap(err, "creating layer writer factory")
		}

		var packageBuilderOpts []buildpack.PackageBuilderOption
		if opts.Flatten {
			packageBuilderOpts = append(packageBuilderOpts, buildpack.DoNotFlatten(opts.FlattenExclude),
				buildpack.WithLayerWriterFactory(writerFactory), buildpack.WithLogger(c.logger))
		}
		packageBuilder := buildpack.NewBuilder(c.imageFactory, packageBuilderOpts...)

		bpURI := opts.Config.Buildpack.URI
		if bpURI == "" {
			return errors.New("buildpack URI must be provided")
		}

		// We need to calculate the relative base directory
		relativeBaseDir := opts.RelativeBaseDir
		if ok, platformRootFolder := buildpack.PlatformRootFolder(relativeBaseDir, target, ""); ok {
			relativeBaseDir = platformRootFolder
		}

		mainBlob, err := c.downloadBuildpackFromURI(ctx, bpURI, relativeBaseDir)
		if err != nil {
			return err
		}

		bp, err := buildpack.FromBuildpackRootBlob(mainBlob, writerFactory)
		if err != nil {
			return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bpURI))
		}

		packageBuilder.SetBuildpack(bp)

		platform := target.OS
		if target.Arch != "" {
			if target.ArchVariant != "" {
				platform = fmt.Sprintf("%s/%s/%s", platform, target.Arch, target.ArchVariant)
			} else {
				platform = fmt.Sprintf("%s/%s", platform, target.Arch)
			}
		}

		for _, dep := range opts.Config.Dependencies {
			depURI := dep.URI
			fileToClean, err := buildpack.PrepareDependencyConfigFile(relativeBaseDir, dep.URI, target, "", multiArch)
			if err != nil {
				return err
			}
			if fileToClean != "" {
				defer os.Remove(fileToClean)
				depURI = filepath.Dir(fileToClean)
			}
			c.logger.Debugf("Downloading buildpack dependency for platform %s", platform)
			mainBP, deps, err := c.buildpackDownloader.Download(ctx, depURI, buildpack.DownloadOptions{
				RegistryName:    opts.Registry,
				RelativeBaseDir: relativeBaseDir,
				ImageOS:         target.OS,
				Platform:        platform,
				ImageName:       dep.ImageName,
				Daemon:          !opts.Publish,
				PullPolicy:      opts.PullPolicy,
				Target:          &target,
				Multiarch:       multiArch,
			})
			if err != nil {
				return errors.Wrapf(err, "packaging dependencies (uri=%s,image=%s)", style.Symbol(dep.URI), style.Symbol(dep.ImageName))
			}

			packageBuilder.AddDependencies(mainBP, deps)
		}

		switch opts.Format {
		case FormatFile:
			name := opts.Name
			if multiArch {
				extension := filepath.Ext(name)
				origFileName := name[:len(name)-len(filepath.Ext(name))]
				if target.Arch != "" {
					name = fmt.Sprintf("%s-%s-%s%s", origFileName, target.OS, target.Arch, extension)
				} else {
					name = fmt.Sprintf("%s-%s%s", origFileName, target.OS, extension)
				}
			}
			err = packageBuilder.SaveAsFile(name, target, opts.Labels)
			if err != nil {
				return err
			}
		case FormatImage:
			img, err := packageBuilder.SaveAsImage(opts.Name, opts.Publish, target, opts.Labels, multiArch)
			if err != nil {
				return errors.Wrapf(err, "saving image")
			}
			if multiArch {
				// We need to keep the identifier to create the image index
				id, err := img.Identifier()
				if err != nil {
					return errors.Wrapf(err, "determining image manifest digest")
				}
				digests = append(digests, id.String())
			}
		default:
			return errors.Errorf("unknown format: %s", style.Symbol(opts.Format))
		}
	}

	if multiArch && len(digests) > 1 {
		return c.CreateManifest(ctx, CreateManifestOptions{
			ManifestName: opts.Name,
			Manifests:    digests,
			Publish:      true,
		})
	}

	return nil
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

func (c *Client) daemonTarget(ctx context.Context, targets []dist.Target) (dist.Target, error) {
	info, err := c.docker.ServerVersion(ctx)
	if err != nil {
		return dist.Target{}, err
	}

	for _, t := range targets {
		if t.OS == info.Os && t.Arch == info.Arch {
			return t, nil
		}
	}
	return dist.Target{}, errors.Errorf("could not find a target that matches daemon os=%s and architecture=%s", info.Os, info.Arch)
}
