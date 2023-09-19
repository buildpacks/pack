package commands

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
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
	Label             map[string]string
	Publish           bool
}

// BuildpackPackager packages buildpacks
type BuildpackPackager interface {
	PackageBuildpack(ctx context.Context, options client.PackageBuildpackOptions) error
}

// PackageConfigReader reads BuildpackPackage configs
type PackageConfigReader interface {
	Read(path string) (pubbldpkg.Config, error)
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
			bpPackageCfg := pubbldpkg.DefaultConfig()
			var bpPath string
			if flags.Path != "" {
				if bpPath, err = filepath.Abs(flags.Path); err != nil {
					return errors.Wrap(err, "resolving buildpack path")
				}
				bpPackageCfg.Buildpack.URI = bpPath
			}
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
			name := args[0]
			if flags.Format == client.FormatFile {
				switch ext := filepath.Ext(name); ext {
				case client.CNBExtension:
				case "":
					name += client.CNBExtension
				default:
					logger.Warnf("%s is not a valid extension for a packaged buildpack. Packaged buildpacks must have a %s extension", style.Symbol(ext), style.Symbol(client.CNBExtension))
				}
			}

			if err := packager.PackageBuildpack(cmd.Context(), client.PackageBuildpackOptions{
				RelativeBaseDir: relativeBaseDir,
				Name:            name,
				Format:          flags.Format,
				Config:          bpPackageCfg,
				Publish:         flags.Publish,
				PullPolicy:      pullPolicy,
				Registry:        flags.BuildpackRegistry,
				Labels:          flags.Label,
			}); err != nil {
				return err
			}

			action := "created"
			if flags.Publish {
				action = "published"
			}
			location := "docker daemon"
			if flags.Format == client.FormatFile {
				location = "file"
			}

			logger.Infof("Successfully %s package %s and saved to %s", action, style.Symbol(name), location)
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.PackageTomlPath, "config", "c", "", "Path to package TOML config")
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "", `Format to save package as ("image" or "file")`)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish to registry (applies to "--format=image" only)`)
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")
	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Path to the Buildpack that needs to be packaged")
	cmd.Flags().StringVarP(&flags.BuildpackRegistry, "buildpack-registry", "r", "", "Buildpack Registry name")
	cmd.Flags().StringToStringVarP(&flags.Label, "label", "l", nil, "Labels to add to packaged Buildpack, in the form of '<name>=<value>'")
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
	return nil
}
