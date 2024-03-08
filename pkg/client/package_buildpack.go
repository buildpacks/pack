package client

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/index"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/blob"
	"github.com/buildpacks/pack/pkg/buildpack"

	"github.com/buildpacks/pack/buildpackage"
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

	// List of buildpack images to exclude from the package been flatten.
	FlattenExclude []string

	// Map of labels to add to the Buildpack
	Labels map[string]string

	// Image Version
	Version string

	// Index Options instruct how IndexManifest should be created
	IndexOptions buildpackage.IndexOptions
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

	var packageBuilderOpts []buildpack.PackageBuilderOption
	if opts.Flatten {
		packageBuilderOpts = append(packageBuilderOpts, buildpack.DoNotFlatten(opts.FlattenExclude),
			buildpack.WithLayerWriterFactory(writerFactory), buildpack.WithLogger(c.logger))
	}
	packageBuilder := buildpack.NewBuilder(c.imageFactory, c.indexFactory, packageBuilderOpts...)

	bpURI := opts.Config.Buildpack.URI
	if bpURI == "" {
		return errors.New("buildpack URI must be provided")
	}

	mainBlob, err := c.downloadBuildpackFromURI(ctx, bpURI, opts.RelativeBaseDir)
	if err != nil {
		return err
	}

	bp, err := buildpack.FromBuildpackRootBlob(mainBlob, writerFactory)
	if err != nil {
		return errors.Wrapf(err, "creating buildpack from %s", style.Symbol(bpURI))
	}

	packageBuilder.SetBuildpack(bp)

	for _, dep := range opts.Config.Dependencies {
		mainBP, deps, err := c.buildpackDownloader.Download(ctx, dep.URI, buildpack.DownloadOptions{
			RegistryName:    opts.Registry,
			RelativeBaseDir: opts.RelativeBaseDir,
			ImageOS:         opts.Config.Platform.OS,
			ImageName:       dep.ImageName,
			Daemon:          !opts.Publish,
			PullPolicy:      opts.PullPolicy,
		})

		if err != nil {
			return errors.Wrapf(err, "packaging dependencies (uri=%s,image=%s)", style.Symbol(dep.URI), style.Symbol(dep.ImageName))
		}

		packageBuilder.AddDependencies(mainBP, deps)
	}

	switch opts.Format {
	case FormatFile:
		// FIXME: Add `variant`, `features`, `osFeatures`, `urls`, `annotations` to imgutil.Platform
		return packageBuilder.SaveAsFile(opts.Name, opts.Version, opts.IndexOptions.Target, opts.Labels)
	case FormatImage:
		// FIXME: Add `variant`, `features`, `osFeatures`, `urls`, `annotations` to imgutil.Platform
		_, err = packageBuilder.SaveAsImage(opts.Name, opts.Version, opts.Publish, opts.IndexOptions.Target, opts.Labels)
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

// PackageBuildpack packages multiple buildpack(s) into image index with each buildpack into either an image or file.
func (c *Client) PackageMultiArchBuildpack(ctx context.Context, opts PackageBuildpackOptions) error {
	if opts.IndexOptions.BPConfigs == nil && len(*opts.IndexOptions.BPConfigs) == 0 {
		return errors.Errorf("%s must not be nil", style.Symbol("IndexOptions"))
	}

	if opts.IndexOptions.PkgConfig == nil {
		return errors.Errorf("package configaration is undefined")
	}

	IndexManifestFn := getIndexManifestFn(c, opts.IndexOptions.Manifest)
	bpCfg, err := buildpackage.NewConfigReader().ReadBuildpackDescriptor(opts.RelativeBaseDir)
	if err != nil {
		return err
	}

	var repoName string
	if info := bpCfg.WithInfo; info.Version == "" {
		repoName = info.ID
	} else {
		repoName = info.ID + ":" + info.Version
	}

	if err := createImageIndex(c, repoName); err != nil {
		return err
	}

	pkgConfig, bpConfigs := *opts.IndexOptions.PkgConfig, *opts.IndexOptions.BPConfigs
	for _, bpConfig := range bpConfigs {
		if err := bpConfig.CopyBuildpackToml(IndexManifestFn); err != nil {
			return err
		}
		defer bpConfig.CleanBuildpackToml()

		targets := bpConfig.Targets()
		if bpConfig.BuildpackType() != pubbldpkg.Composite {
			target := targets[0]
			distro := target.Distributions[0]
			if err := pkgConfig.CopyPackageToml(opts.IndexOptions.RelativeBaseDir, target, distro, distro.Versions[0], IndexManifestFn); err != nil {
				return err
			}
			defer pkgConfig.CleanPackageToml(opts.IndexOptions.RelativeBaseDir, target, distro.Name, distro.Versions[0])
		}

		if !opts.Flatten && bpConfig.Flatten {
			opts.IndexOptions.Logger.Warn("Flattening a buildpack package could break the distribution specification. Please use it with caution.")
		}

		if err := c.PackageBuildpack(ctx, PackageBuildpackOptions{
			RelativeBaseDir: bpConfig.RelativeBaseDir(),
			Name:            opts.Name,
			Format:          opts.Format,
			Config:          pkgConfig.Config,
			Publish:         opts.Publish,
			PullPolicy:      opts.PullPolicy,
			Registry:        opts.Registry,
			Flatten:         bpConfig.Flatten,
			FlattenExclude:  bpConfig.FlattenExclude,
			Labels:          bpConfig.Labels,
			Version:         opts.Version,
		}); err != nil {
			return err
		}
	}
	return nil
}

func getIndexManifestFn(c *Client, mfest *v1.IndexManifest) func(ref name.Reference) (*v1.IndexManifest, error) {
	IndexHandlerFn := func(ref name.Reference) (*v1.IndexManifest, error) {
		if mfest != nil {
			return mfest, nil
		}

		fetchOpts, err := withOptions([]index.Option{
			index.WithInsecure(true),
		}, c.keychain)
		if err != nil {
			return nil, err
		}

		idx, err := c.indexFactory.FetchIndex(ref.Name(), fetchOpts...)
		if err != nil {
			return nil, err
		}

		ii, ok := idx.(*imgutil.ManifestHandler)
		if !ok {
			return nil, errors.Errorf("unknown handler: %s", style.Symbol("ManifestHandler"))
		}

		return ii.IndexManifest()
	}

	return IndexHandlerFn
}

func createImageIndex(c *Client, repoName string) (err error) {
	opts, err := withOptions([]index.Option{
		index.WithInsecure(true),
	}, c.keychain)
	if err != nil {
		return err
	}

	// Delete ImageIndex if already exists
	if idx, err := c.indexFactory.LoadIndex(repoName, opts...); err == nil {
		if err = idx.Delete(); err != nil {
			return err
		}
	}

	// Create a new ImageIndex. the newly created index has `Image(hash v1.Hash)` `ImageIndex(hash v1.Hash)` that always returns error.
	_, err = c.indexFactory.CreateIndex(repoName, opts...)
	return err
}

func loadImageIndex(c *Client, repoName string) (imgutil.ImageIndex, error) {
	opts, err := withOptions([]index.Option{
		index.WithInsecure(true),
	}, c.keychain)
	if err != nil {
		return nil, err
	}

	return c.indexFactory.LoadIndex(repoName, opts...)
}
