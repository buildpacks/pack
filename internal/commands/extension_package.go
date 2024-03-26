package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/target"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

// ExtensionPackageFlags define flags provided to the ExtensionPackage command
type ExtensionPackageFlags struct {
	PackageTomlPath string
	Format          string
	Publish         bool
	Policy          string
	Targets         []string
}

// ExtensionPackager packages extensions
type ExtensionPackager interface {
	PackageExtension(ctx context.Context, options client.PackageBuildpackOptions) error
	PackageMultiArchExtension(ctx context.Context, opts client.PackageBuildpackOptions) error
}

// ExtensionPackage packages (a) extension(s) into OCI format, based on a package config
func ExtensionPackage(logger logging.Logger, cfg config.Config, packager ExtensionPackager, packageConfigReader PackageConfigReader) *cobra.Command {
	var flags ExtensionPackageFlags
	cmd := &cobra.Command{
		Use:   "package <name> --config <config-path>",
		Short: "Package an extension in OCI format",
		Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateExtensionPackageFlags(&flags); err != nil {
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

			exPackageCfg := pubbldpkg.DefaultExtensionConfig()
			relativeBaseDir := ""
			if flags.PackageTomlPath != "" {
				exPackageCfg, err = packageConfigReader.Read(flags.PackageTomlPath)
				if err != nil {
					return errors.Wrap(err, "reading config")
				}

				relativeBaseDir, err = filepath.Abs(filepath.Dir(flags.PackageTomlPath))
				if err != nil {
					return errors.Wrap(err, "getting absolute path for config")
				}
			}

			pkgMultiArchConfig := pubbldpkg.NewMultiArchPackage(exPackageCfg, relativeBaseDir)
			var extPath = "."
			if extPath, err = filepath.Abs("."); err != nil {
				return errors.Wrap(err, "resolving extension path")
			}
			exPackageCfg.Buildpack.URI = extPath

			extConfigPathAbs, err := filepath.Abs(extPath)
			if err != nil {
				return err
			}
			extConfigPath := filepath.Join(extConfigPathAbs, "extension.toml")
			if _, err = os.Stat(extConfigPath); err != nil {
				return fmt.Errorf("cannot find %s: %s", style.Symbol("extension.toml"), style.Symbol(extConfigPath))
			}

			extConfig, err := packageConfigReader.ReadExtensionDescriptor(extConfigPath)
			if err != nil {
				return err
			}
			extMultiArchConfig := pubbldpkg.NewMultiArchExtension(extConfig, extPath, targets)
			extConfigs, err := extMultiArchConfig.MultiArchConfigs()
			if err != nil {
				return err
			}

			name := args[0]
			if flags.Format == client.FormatFile {
				switch ext := filepath.Ext(name); ext {
				case client.CNBExtension:
				case "":
					name += client.CNBExtension
				default:
					logger.Warnf("%s is not a valid extension for a packaged extension. Packaged extensions must have a %s extension", style.Symbol(ext), style.Symbol(client.CNBExtension))
				}
			}

			pkgExtOpts := client.PackageBuildpackOptions{
				RelativeBaseDir: relativeBaseDir,
				Name:            name,
				Format:          flags.Format,
				Config:          exPackageCfg,
				Publish:         flags.Publish,
				PullPolicy:      pullPolicy,
			}

			if len(extConfigs) > 1 {
				// if _, err := os.Stat(filepath.Join(extPath, "extension.toml")); err != nil {
				// 	return fmt.Errorf("cannot open %s", extPath)
				// extPath = filepath.Join(extPath, "extension.toml")
				// } else if f.IsDir() {
				// extPath = filepath.Join(f.Name(), "extension.toml")
				// }

				pkgExtOpts.RelativeBaseDir = extConfigPath
				pkgExtOpts.IndexOptions = pubbldpkg.IndexOptions{
					ExtConfigs: &extConfigs,
					PkgConfig:  pkgMultiArchConfig,
					Logger:     logger,
				}

				if err := packager.PackageMultiArchExtension(cmd.Context(), pkgExtOpts); err != nil {
					if err := revertExtConfig(extConfigPath, extConfig); err != nil {
						return fmt.Errorf("unable to revert changes of extention %s", style.Symbol(name))
					}
					return err
				}

				if err := revertExtConfig(extConfigPath, extConfig); err != nil {
					return fmt.Errorf("unable to revert changes of extention %s", style.Symbol(name))
				}
			} else {
				if len(extConfigs) == 1 {
					pkgExtOpts.IndexOptions.Target = extConfigs[0].WithTargets[0]
				}

				if err := packager.PackageExtension(cmd.Context(), pkgExtOpts); err != nil {
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
			logger.Infof("Successfully %s package %s and saved to %s", action, style.Symbol(name), location)
			return nil
		}),
	}

	// flags will be added here
	cmd.Flags().StringVarP(&flags.PackageTomlPath, "config", "c", "", "Path to package TOML config")
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "", `Format to save package as ("image" or "file")`)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish the extension directly to the container registry specified in <name>, instead of the daemon (applies to "--format=image" only).`)
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")
	cmd.Flags().StringSliceVarP(&flags.Targets, "target", "t", nil,
		`Targets are the platforms list to build. one can provide target platforms in format [os][/arch][/variant]:[distroname@osversion@anotherversion];[distroname@osversion]
	- Base case for two different architectures :  '--target "linux/amd64" --target "linux/arm64"'
	- case for distribution version: '--target "windows/amd64:windows-nano@10.0.19041.1415"'
	- case for different architecture with distributed versions : '--target "linux/arm/v6:ubuntu@14.04"  --target "linux/arm/v6:ubuntu@16.04"'
	`)
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("target")
	}
	AddHelpFlag(cmd, "package")
	return cmd
}

func revertExtConfig(extConfigPath string, extConfig dist.ExtensionDescriptor) error {
	extConfigFile, err := os.OpenFile(extConfigPath, os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}

	return toml.NewEncoder(extConfigFile).Encode(extConfig)
}

func validateExtensionPackageFlags(p *ExtensionPackageFlags) error {
	if p.Publish && p.Policy == image.PullNever.String() {
		return errors.Errorf("--publish and --pull-policy=never cannot be used together. The --publish flag requires the use of remote images.")
	}
	return nil
}
