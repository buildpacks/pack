package client

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/index"

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

	// List of buildpack images to exclude from the package been flatten.
	FlattenExclude []string

	// Map of labels to add to the Buildpack
	Labels map[string]string

	// Image Version
	Version string

	// Index Options instruct how IndexManifest should be created
	IndexOptions pubbldpkg.IndexOptions
}

// PackageBuildpack packages buildpack(s) into either an image or file.
func (c *Client) PackageBuildpack(ctx context.Context, opts PackageBuildpackOptions) error {
	if opts.Format == "" {
		opts.Format = FormatImage
	}

	if opts.Config.Platform.OS == "windows" && !c.experimental {
		return NewExperimentError("Windows buildpackage support is currently experimental.")
	}

	if err := c.validateOSPlatform(ctx, opts.Config.Platform.OS, opts.Publish, opts.Format); err != nil {
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

	if opts.IndexOptions.ImageIndex != nil && opts.Format == FormatFile {
		return packageBuilder.SaveAsMultiArchFile(opts.Name, opts.Version, opts.IndexOptions.Targets, opts.IndexOptions.ImageIndex, opts.Labels)
	}

	switch opts.Format {
	case FormatFile:
		return packageBuilder.SaveAsFile(opts.Name, opts.Version, opts.IndexOptions.Targets[0], opts.IndexOptions.ImageIndex, opts.Labels)
	case FormatImage:
		if _, err = packageBuilder.SaveAsImage(opts.Name, opts.Version, opts.Publish, opts.IndexOptions.Targets[0], opts.IndexOptions.ImageIndex, opts.Labels); err == nil {
			version := opts.Version
			if version == "" {
				version = "latest"
			}
			fmt.Println("Successfully saved image: " + opts.Name + ":" + version)
			return nil
		}
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
	if !c.experimental {
		return errors.Errorf("packaging %s is currently %s", style.Symbol("multi arch buildpacks"), style.Symbol(("experimental")))
	}

	if opts.IndexOptions.BPConfigs == nil || len(*opts.IndexOptions.BPConfigs) < 2 {
		return errors.Errorf("%s must not be nil", style.Symbol("IndexOptions"))
	}

	if opts.IndexOptions.PkgConfig == nil {
		return errors.Errorf("package configaration is undefined")
	}

	bpCfg, err := pubbldpkg.NewConfigReader().ReadBuildpackDescriptor(opts.RelativeBaseDir)
	if err != nil {
		return fmt.Errorf("cannot read %s file: %s", style.Symbol("buildpack.toml"), style.Symbol(opts.RelativeBaseDir))
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
	var (
		errs errgroup.Group
	)

	idx, err := loadImageIndex(c, repoName)
	if err != nil {
		return err
	}

	IndexManifestFn := c.GetIndexManifestFn()
	packageMultiArchBuildpackFn := func(bpConfig pubbldpkg.MultiArchBuildpackConfig) error {
		if err := bpConfig.CopyBuildpackToml(IndexManifestFn); err != nil {
			return err
		}
		defer bpConfig.CleanBuildpackToml()

		if targets := bpConfig.Targets(); bpConfig.BuildpackType() != pubbldpkg.Composite {
			target, distro, version := dist.Target{}, dist.Distribution{}, ""
			if len(targets) != 0 {
				target = targets[0]
			}

			if len(target.Distributions) != 0 {
				distro = target.Distributions[0]
			}

			if len(distro.Versions) != 0 {
				version = distro.Versions[0]
			}

			if err := pkgConfig.CopyPackageToml(filepath.Dir(bpConfig.Path()), target, distro.Name, version, IndexManifestFn); err != nil {
				return err
			}
			defer pkgConfig.CleanPackageToml(filepath.Dir(bpConfig.Path()), target, distro.Name, version)
		}

		if !opts.Flatten && bpConfig.Flatten {
			opts.IndexOptions.Logger.Warn("Flattening a buildpack package could break the distribution specification. Please use it with caution.")
		}

		return c.PackageBuildpack(ctx, PackageBuildpackOptions{
			RelativeBaseDir: "",
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
			IndexOptions: pubbldpkg.IndexOptions{
				ImageIndex: idx,
				Logger:     opts.IndexOptions.Logger,
				Targets:    bpConfig.Targets(),
			},
		})
	}

	for _, bpConfig := range bpConfigs {
		c := bpConfig
		errs.Go(func() error {
			return packageMultiArchBuildpackFn(c)
		})
	}

	if err := errs.Wait(); err != nil {
		return err
	}

	if err := idx.Save(); err != nil {
		return err
	}

	if !opts.Publish {
		return nil
	}

	return idx.Push(imgutil.WithInsecure(true), imgutil.WithTags("latest"))
}

func (c *Client) GetIndexManifestFn() pubbldpkg.GetIndexManifestFn {
	if len(c.cachedIndexManifests) == 0 {
		c.cachedIndexManifests = make(map[name.Reference]*v1.IndexManifest)
	}

	IndexHandlerFn := func(ref name.Reference) (*v1.IndexManifest, error) {
		mfest := c.cachedIndexManifests[ref]
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
			return nil, errors.Errorf("the given reference(%s) either doesn't exist or not referencing IndexManifest", style.Symbol(ref.Name()))
		}

		ii, ok := idx.(*imgutil.ManifestHandler)
		if !ok {
			return nil, errors.Errorf("unknown handler: %s", style.Symbol("ManifestHandler"))
		}

		if mfest, err = ii.IndexManifest(); err != nil {
			return mfest, err
		}

		c.cachedIndexManifests[ref] = mfest
		return mfest, nil
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
