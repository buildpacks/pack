package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/target"
	"github.com/buildpacks/pack/pkg/client"
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
}

// PackageConfigReader reads BuildpackPackage configs
type PackageConfigReader interface {
	Read(path string) (pubbldpkg.Config, error)
}

const (
	Buildpack = "buildpack.toml"
	Package   = "package.toml"
)

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
			var (
				bpPackageCfg                           pubbldpkg.Config
				bpPath                                 string
				relativeBaseDir                        string
				bpConfigs                              []pubbldpkg.Config
				isMultiArch                            bool
				from                                   BPTargetType
				packageTomlConfig, buildpackTomlConfig pubbldpkg.Config
				name                                   = args[0]
			)

			targets, err := target.ParseTargets(flags.Targets, logger)
			if err != nil {
				return err
			}

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

			if flags.Path != "" {
				if bpPath, err = filepath.Abs(flags.Path); err != nil {
					return errors.Wrap(err, "resolving buildpack path")
				}
			}

			packageBuildpackFn := func() error {
				if flags.Path != "" {
					bpPackageCfg.Buildpack.URI = bpPath
				}

				if flags.Flatten {
					bpPackageCfg.Flatten = true
				}

				if len(flags.FlattenExclude) != 0 {
					bpPackageCfg.FlattenExclude = flags.FlattenExclude
				}

				if len(flags.Label) != 0 {
					bpPackageCfg.Labels = flags.Label
				}

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

				if flags.Format == client.FormatFile {
					switch ext := filepath.Ext(name); ext {
					case client.CNBExtension:
					case "":
						name += client.CNBExtension
					default:
						logger.Warnf("%s is not a valid extension for a packaged buildpack. Packaged buildpacks must have a %s extension", style.Symbol(ext), style.Symbol(client.CNBExtension))
					}
				}
				if flags.Flatten {
					logger.Warn("Flattening a buildpack package could break the distribution specification. Please use it with caution.")
				}

				return packager.PackageBuildpack(cmd.Context(), client.PackageBuildpackOptions{
					RelativeBaseDir: relativeBaseDir,
					Name:            name,
					Format:          flags.Format,
					Config:          bpPackageCfg,
					Publish:         flags.Publish,
					PullPolicy:      pullPolicy,
					Registry:        flags.BuildpackRegistry,
					Flatten:         bpPackageCfg.Flatten,
					FlattenExclude:  bpPackageCfg.FlattenExclude,
					Labels:          bpPackageCfg.Labels,
				})
			}

			path := "."
			if flags.PackageTomlPath != "" {
				path = flags.PackageTomlPath
			}

			packageTomlFilePath := filepath.Join(path, Package)
			if _, err := os.Stat(packageTomlFilePath); err == nil {
				bpPackageCfg, err = packageConfigReader.Read(packageTomlFilePath)
				if err != nil {
					return err
				}

				packageTomlConfig = bpPackageCfg
				bpConfigs = pubbldpkg.MultiArchDefaultConfigs(bpPackageCfg.Target)
				isMultiArch, from = len(bpConfigs) > 1, PackageToml
			}

			path = "."
			if flags.Path != "" {
				path = bpPath
			}

			buildpackTomlFilePath := filepath.Join(path, Buildpack)
			if _, err := os.Stat(buildpackTomlFilePath); err == nil {
				buildpackTomlConfig, err = packageConfigReader.Read(filepath.Join(path, Buildpack))
				if err != nil {
					return err
				}
			}

			if !isMultiArch {
				bpPackageCfg = buildpackTomlConfig
				bpConfigs = pubbldpkg.MultiArchDefaultConfigs(bpPackageCfg.Target)
				from = BuildpackToml
			}

			if len(flags.Targets) > 0 {
				bpConfigs = pubbldpkg.MultiArchDefaultConfigs(targets)
				from = Flags
			}

			switch len(bpConfigs) {
			case 0:
				bpPackageCfg = pubbldpkg.DefaultConfig()
				if err := packageBuildpackFn(); err != nil {
					return err
				}
			case 1:
				bpPackageCfg = bpConfigs[0]
				bpPackageCfg.Buildpack.URI = "."
				if err := packageBuildpackFn(); err != nil {
					return err
				}
			default:
				if flags.PackageTomlPath != "" {
					relativeBaseDir, err = filepath.Abs(filepath.Dir(flags.PackageTomlPath))
					if err != nil {
						return errors.Wrap(err, "getting absolute path for config")
					}
				}

				if flags.Format == client.FormatFile {
					switch ext := filepath.Ext(name); ext {
					case client.CNBExtension:
					case "":
						name += client.CNBExtension
					default:
						logger.Warnf("%s is not a valid extension for a packaged buildpack. Packaged buildpacks must have a %s extension", style.Symbol(ext), style.Symbol(client.CNBExtension))
					}
				}

				for _, bpConfig := range bpConfigs {
					dupBPConfig := buildpackTomlConfig
					dupPkgConfig := packageTomlConfig

					if dupBPConfig.Buildpack.URI == "" {
						dupBPConfig = pubbldpkg.DefaultConfig()
					}
					dupBPConfig.Buildpack.URI = "."

					if dupPkgConfig.Buildpack.URI == "" && dupPkgConfig.Extension.URI == "" {
						dupPkgConfig, err = packageConfigReader.Read(packageTomlFilePath)
						if err != nil {
							return err
						}
					}

					bpFilePath := filepath.Join(buildpackTomlConfig.Buildpack.URI, Buildpack)
					bpFile, err := os.OpenFile(bpFilePath, os.O_WRONLY|os.O_CREATE, 0644)
					if err != nil {
						return err
					}

					if err := toml.NewEncoder(bpFile).Encode(dupBPConfig); err != nil {
						return err
					}

					pkgFilePath := filepath.Join(buildpackTomlConfig.Buildpack.URI, Package)
					pkgFile, err := os.OpenFile(pkgFilePath, os.O_WRONLY|os.O_CREATE, 0644)
					if err != nil {
						return err
					}

					if err := toml.NewEncoder(pkgFile).Encode(dupPkgConfig); err != nil {
						return err
					}

					defer func() {
						bpFile.Close()
						pkgFile.Close()
						os.Remove(bpFilePath)
						os.Remove(pkgFilePath)
					}()

					if flags.Flatten {
						logger.Warn("Flattening a buildpack package could break the distribution specification. Please use it with caution.")
						bpConfig.Flatten = true
					}

					if len(flags.FlattenExclude) > 0 {
						logger.Warnf("Cannot use %s flag for MultiArch Targets. Please define %s in %s file", style.Symbol("--flatten-exclude"), style.Symbol("flatten.exclude"), style.Symbol(from.String()))
					}

					if len(flags.Label) != 0 {
						logger.Warnf("Using %s along with %s defined in %s", style.Symbol("--labels"), style.Symbol("labels table"), style.Symbol(from.String()))
						for k, v := range flags.Label {
							bpConfig.Labels[k] = v
						}
					}

					if err := packager.PackageBuildpack(cmd.Context(), client.PackageBuildpackOptions{
						RelativeBaseDir: relativeBaseDir,
						Name:            name,
						Format:          flags.Format,
						Config:          bpConfig,
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
			logger.Infof("Successfully %s package %s and saved to %s", action, style.Symbol(name), location)
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
	cmd.Flags().StringSliceVarP(&flags.Targets, "target", "t", nil,
		`Targets are the platforms list to build. one can provide target platforms in format [os][/arch][/variant]:[distroname@osversion@anotherversion];[distroname@osversion]
	- Base case for two different architectures :  '--target "linux/amd64" --target "linux/arm64"'
	- case for distribution version: '--target "windows/amd64:windows-nano@10.0.19041.1415"'
	- case for different architecture with distributed versions : '--target "linux/arm/v6:ubuntu@14.04"  --target "linux/arm/v6:ubuntu@16.04"'
	`)
	cmd.Flags().StringSliceVarP(&flags.FlattenExclude, "flatten-exclude", "e", nil, "Buildpacks to exclude from flattening, in the form of '<buildpack-id>@<buildpack-version>'")
	cmd.Flags().StringToStringVarP(&flags.Label, "label", "l", nil, "Labels to add to packaged Buildpack, in the form of '<name>=<value>'")
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

type BPTargetType int

const (
	// `[]dist.Target` took from pack cli
	Flags BPTargetType = iota
	// `[]dist.Target` took from `buildpack.toml` file
	BuildpackToml
	// `[]dist.Target` took from `package.toml` file
	PackageToml
)

func (t BPTargetType) String() string {
	switch t {
	case PackageToml:
		return Package
	default:
		return Buildpack
	}
}
