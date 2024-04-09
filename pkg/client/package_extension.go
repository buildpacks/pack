package client

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/imgutil"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/layer"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
)

// PackageExtension packages extension(s) into either an image or file.
func (c *Client) PackageExtension(ctx context.Context, opts PackageBuildpackOptions) error {
	if opts.Format == "" {
		opts.Format = FormatImage
	}

	if opts.Config.Platform.OS == "windows" && !c.experimental {
		return NewExperimentError("Windows extensionpackage support is currently experimental.")
	}

	if err := c.validateOSPlatform(ctx, opts.Config.Platform.OS, opts.Publish, opts.Format); err != nil {
		return err
	}

	writerFactory, err := layer.NewWriterFactory(opts.Config.Platform.OS)
	if err != nil {
		return errors.Wrap(err, "creating layer writer factory")
	}

	packageBuilder := buildpack.NewBuilder(c.imageFactory, c.indexFactory)

	exURI := opts.Config.Extension.URI
	if exURI == "" {
		return errors.New("extension URI must be provided")
	}

	mainBlob, err := c.downloadBuildpackFromURI(ctx, exURI, opts.RelativeBaseDir)
	if err != nil {
		return err
	}

	ex, err := buildpack.FromExtensionRootBlob(mainBlob, writerFactory)
	if err != nil {
		return errors.Wrapf(err, "creating extension from %s", style.Symbol(exURI))
	}

	packageBuilder.SetExtension(ex)

	if opts.Format == FormatFile && opts.IndexOptions.ImageIndex != nil {
		return packageBuilder.SaveAsMultiArchFile(opts.Name, opts.Version, opts.IndexOptions.Targets, opts.IndexOptions.ImageIndex, make(map[string]string))
	}
	switch opts.Format {
	case FormatFile:
		return packageBuilder.SaveAsFile(opts.Name, opts.Version, opts.IndexOptions.Targets[0], opts.IndexOptions.ImageIndex, map[string]string{})
	case FormatImage:
		_, err = packageBuilder.SaveAsImage(opts.Name, opts.Version, opts.Publish, opts.IndexOptions.Targets[0], opts.IndexOptions.ImageIndex, map[string]string{})
		return errors.Wrapf(err, "saving image")
	default:
		return errors.Errorf("unknown format: %s", style.Symbol(opts.Format))
	}
}

func (c *Client) PackageMultiArchExtension(ctx context.Context, opts PackageBuildpackOptions) error {
	if !c.experimental {
		return errors.Errorf("packaging %s is currently %s", style.Symbol("multi arch extensions"), style.Symbol(("experimental")))
	}

	if opts.IndexOptions.ExtConfigs == nil || len(*opts.IndexOptions.ExtConfigs) < 2 {
		return errors.Errorf("%s must not be nil", style.Symbol("IndexOptions"))
	}

	if opts.IndexOptions.PkgConfig == nil {
		return errors.Errorf("package configaration is undefined")
	}

	extCfg, err := pubbldpkg.NewConfigReader().ReadExtensionDescriptor(opts.RelativeBaseDir)
	if err != nil {
		return fmt.Errorf("cannot read %s file: %s", style.Symbol("extension.toml"), style.Symbol(opts.RelativeBaseDir))
	}

	var repoName string
	if info := extCfg.WithInfo; info.Version == "" {
		repoName = info.ID
	} else {
		repoName = info.ID + ":" + info.Version
	}

	if err := createImageIndex(c, repoName); err != nil {
		return err
	}

	pkgConfig, extConfigs := *opts.IndexOptions.PkgConfig, *opts.IndexOptions.ExtConfigs

	var errs errgroup.Group
	idx, err := loadImageIndex(c, repoName)
	if err != nil {
		return err
	}

	IndexManifestFn := c.GetIndexManifestFn()
	packageMultiArchExtensionFn := func(extConfig pubbldpkg.MultiArchExtensionConfig) error {
		if err := extConfig.CopyExtensionToml(IndexManifestFn); err != nil {
			return err
		}
		defer extConfig.CleanExtensionToml()

		targets, target, distro, version := extConfig.Targets(), dist.Target{}, dist.Distribution{}, ""
		if len(targets) != 0 {
			target = targets[0]
		}

		if len(target.Distributions) != 0 {
			distro = target.Distributions[0]
		}

		if len(distro.Versions) != 0 {
			version = distro.Versions[0]
		}
		if err := pkgConfig.CopyPackageToml(filepath.Dir(extConfig.Path()), target, distro.Name, version, IndexManifestFn); err != nil {
			return err
		}
		defer pkgConfig.CleanPackageToml(filepath.Dir(extConfig.Path()), target, distro.Name, version)

		return c.PackageExtension(ctx, PackageBuildpackOptions{
			RelativeBaseDir: "",
			Name:            opts.Name,
			Format:          opts.Format,
			Config:          pkgConfig.Config,
			Publish:         opts.Publish,
			PullPolicy:      opts.PullPolicy,
			Registry:        opts.Registry,
			Version:         opts.Version,
			IndexOptions: pubbldpkg.IndexOptions{
				ImageIndex: idx,
				Logger:     opts.IndexOptions.Logger,
				Targets:    extConfig.Targets(),
			},
		})
	}

	for _, extConfig := range extConfigs {
		c := extConfig
		errs.Go(func() error {
			return packageMultiArchExtensionFn(c)
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

	return idx.Push(imgutil.WithInsecure(true) /* imgutil.WithTags("latest") */)
}
