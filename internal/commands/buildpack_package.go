package commands

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/index"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/target"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

// BuildpackPackageFlags define flags provided to the BuildpackPackage command
type BuildpackPackageFlags struct {
	PackageTomlPath   string
	Format            string
	Policy            string
	BuildpackRegistry string
	Path              string
	FlattenExclude    []string
	Label             map[string]string
	Publish           bool
	Flatten           bool
	Targets           []string
}

// BuildpackPackager packages buildpacks
type BuildpackPackager interface {
	PackageBuildpack(ctx context.Context, options client.PackageBuildpackOptions) error
	IndexManifest(ctx context.Context, ref name.Reference) (*v1.IndexManifest, error)
	LoadIndex(reponame string, opts ...index.Option) (imgutil.ImageIndex, error)
	CreateIndex(repoName string, opts ...index.Option) (imgutil.ImageIndex, error)
}

// PackageConfigReader reads BuildpackPackage configs
type PackageConfigReader interface {
	Read(path string) (pubbldpkg.Config, error)
	ReadBuildpackDescriptor(path string) (dist.BuildpackDescriptor, error)
}

// BuildpackPackage packages (a) buildpack(s) into OCI format, based on a package config
func BuildpackPackage(logger logging.Logger, cfg config.Config, packager BuildpackPackager, packageConfigReader PackageConfigReader) *cobra.Command {
	var flags BuildpackPackageFlags
	cmd := &cobra.Command{
		Use:     "package <name> --config <config-path>",
		Short:   "Package a buildpack in OCI format.",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Example: "pack buildpack package my-buildpack --config ./package.toml\npack buildpack package my-buildpack.cnb --config ./package.toml --f file",
		Long: "buildpack package allows users to package (a) buildpack(s) into OCI format, which can then to be hosted in " +
			"image repositories or persisted on disk as a '.cnb' file. You can also package a number of buildpacks " +
			"together, to enable easier distribution of a set of buildpacks. " +
			"Packaged buildpacks can be used as inputs to `pack build` (using the `--buildpack` flag), " +
			"and they can be included in the configs used in `pack builder create` and `pack buildpack package`. For more " +
			"on how to package a buildpack, see: https://buildpacks.io/docs/buildpack-author-guide/package-a-buildpack/.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateBuildpackPackageFlags(cfg, &flags); err != nil {
				return err
			}

			stringPolicy := flags.Policy
			if stringPolicy == "" {
				stringPolicy = cfg.PullPolicy
			}
			pullPolicy, err := image.ParsePullPolicy(stringPolicy)
			if err != nil {
				return errors.Wrap(err, "parsing pull policy")
			}

			targets, err := target.ParseTargets(flags.Targets, logger)
			if err != nil {
				return err
			}

			bpPackageCfg := pubbldpkg.DefaultConfig()
			relativeBaseDir := ""
			if flags.PackageTomlPath != "" {
				bpPackageCfg, err = packageConfigReader.Read(flags.PackageTomlPath)
				if err != nil {
					return errors.Wrap(err, "reading config")
				}

				relativeBaseDir, err = filepath.Abs(filepath.Dir(flags.PackageTomlPath))
				if err != nil {
					return errors.Wrap(err, "getting absolute path for config")
				}
			}

			pkgMultiArchConfig := pubbldpkg.NewMultiArchPackage(bpPackageCfg, relativeBaseDir, logger)
			var bpPath string
			if flags.Path != "" {
				if bpPath, err = filepath.Abs(flags.Path); err != nil {
					return errors.Wrap(err, "resolving buildpack path")
				}
				bpPackageCfg.Buildpack.URI = bpPath
			}

			bpConfig, err := packageConfigReader.ReadBuildpackDescriptor(bpPath)
			if err != nil {
				return err
			}
			bpMultiArchConfig := pubbldpkg.NewMultiArchBuildpack(bpConfig, bpPath, flags.Flatten, targets, logger)
			bpConfigs, err := bpMultiArchConfig.MultiArchConfigs()
			if err != nil {
				return err
			}

			bpName := args[0]
			if flags.Format == client.FormatFile {
				switch ext := filepath.Ext(bpName); ext {
				case client.CNBExtension:
				case "":
					bpName += client.CNBExtension
				default:
					logger.Warnf("%s is not a valid extension for a packaged buildpack. Packaged buildpacks must have a %s extension", style.Symbol(ext), style.Symbol(client.CNBExtension))
				}
			}

			if flags.Flatten {
				logger.Warn("Flattening a buildpack package could break the distribution specification. Please use it with caution.")
			}

			var mfest *v1.IndexManifest
			getIndexManifestFn := func(ref name.Reference) (*v1.IndexManifest, error) {
				if mfest != nil {
					return mfest, nil
				}
				return packager.IndexManifest(cmd.Context(), ref)
			}

			// packager.LoadIndex(b)

			if len(bpConfigs) > 0 {
				for _, bpConfig := range bpConfigs {
					if err = bpConfig.CopyBuildpackToml(getIndexManifestFn); err != nil {
						return err
					}
					defer bpConfig.CleanBuildpackToml()

					targets := bpConfig.Targets()
					if bpConfig.BuildpackType() != pubbldpkg.Composite {
						target := targets[0]
						distro := target.Distributions[0]
						if err = pkgMultiArchConfig.CopyPackageToml(bpPath, target, distro, distro.Versions[0], getIndexManifestFn); err != nil {
							return err
						}
						defer pkgMultiArchConfig.CleanPackageToml(bpPath, target, distro, distro.Versions[0])
					}

					if !flags.Flatten && bpConfig.Flatten {
						logger.Warn("Flattening a buildpack package could break the distribution specification. Please use it with caution.")
					}

					if err := packager.PackageBuildpack(cmd.Context(), client.PackageBuildpackOptions{
						RelativeBaseDir: bpConfig.RelativeBaseDir(),
						Name:            bpName,
						Format:          flags.Format,
						Config:          pkgMultiArchConfig.Config(),
						Publish:         flags.Publish,
						PullPolicy:      pullPolicy,
						Registry:        flags.BuildpackRegistry,
						Flatten:         bpConfig.Flatten,
						FlattenExclude:  bpConfig.FlattenExclude,
						Labels:          bpConfig.Labels,
					}); err != nil {
						return err
					}
				}
			} else {
				if err := packager.PackageBuildpack(cmd.Context(), client.PackageBuildpackOptions{
					RelativeBaseDir: relativeBaseDir,
					Name:            bpName,
					Format:          flags.Format,
					Config:          bpPackageCfg,
					Publish:         flags.Publish,
					PullPolicy:      pullPolicy,
					Registry:        flags.BuildpackRegistry,
					Flatten:         flags.Flatten,
					FlattenExclude:  flags.FlattenExclude,
					Labels:          flags.Label,
				}); err != nil {
					return err
				}
			}

			action := "created"
			location := "docker daemon"
			if flags.Publish {
				action = "published"
				location = "registry"
			}
			if flags.Format == client.FormatFile {
				location = "file"
			}
			logger.Infof("Successfully %s package %s and saved to %s", action, style.Symbol(bpName), location)
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.PackageTomlPath, "config", "c", "", "Path to package TOML config")
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "", `Format to save package as ("image" or "file")`)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish the buildpack directly to the container registry specified in <name>, instead of the daemon (applies to "--format=image" only).`)
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")
	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Path to the Buildpack that needs to be packaged")
	cmd.Flags().StringVarP(&flags.BuildpackRegistry, "buildpack-registry", "r", "", "Buildpack Registry name")
	cmd.Flags().BoolVar(&flags.Flatten, "flatten", false, "Flatten the buildpack into a single layer")
	cmd.Flags().StringSliceVarP(&flags.FlattenExclude, "flatten-exclude", "e", nil, "Buildpacks to exclude from flattening, in the form of '<buildpack-id>@<buildpack-version>'")
	cmd.Flags().StringToStringVarP(&flags.Label, "label", "l", nil, "Labels to add to packaged Buildpack, in the form of '<name>=<value>'")
	cmd.Flags().StringSliceVarP(&flags.Targets, "target", "t", nil,
		`Targets are the platforms list to build. one can provide target platforms in format [os][/arch][/variant]:[distroname@osversion@anotherversion];[distroname@osversion]
	- Base case for two different architectures :  '--target "linux/amd64" --target "linux/arm64"'
	- case for distribution version: '--target "windows/amd64:windows-nano@10.0.19041.1415"'
	- case for different architecture with distributed versions : '--target "linux/arm/v6:ubuntu@14.04"  --target "linux/arm/v6:ubuntu@16.04"'
	`)
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("flatten")
		cmd.Flags().MarkHidden("flatten-exclude")
	}
	AddHelpFlag(cmd, "package")
	return cmd
}

func validateBuildpackPackageFlags(cfg config.Config, p *BuildpackPackageFlags) error {
	if p.Publish && p.Policy == image.PullNever.String() {
		return errors.Errorf("--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
	}
	if p.PackageTomlPath != "" && p.Path != "" {
		return errors.Errorf("--config and --path cannot be used together. Please specify the relative path to the Buildpack directory in the package config file.")
	}

	if p.Flatten {
		if !cfg.Experimental {
			return client.NewExperimentError("Flattening a buildpack package is currently experimental.")
		}

		if len(p.FlattenExclude) > 0 {
			for _, exclude := range p.FlattenExclude {
				if strings.Count(exclude, "@") != 1 {
					return errors.Errorf("invalid format %s; please use '<buildpack-id>@<buildpack-version>' to exclude buildpack from flattening", exclude)
				}
			}
		}
	}
	return nil
}
