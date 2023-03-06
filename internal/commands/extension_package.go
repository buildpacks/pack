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

// ExtensionPackageFlags define flags provided to the ExtensionPackage command
type ExtensionPackageFlags struct {
	PackageTomlPath string
	Format          string
	Publish         bool
	Policy          string
}

// ExtensionPackager packages extensions
type ExtensionPackager interface {
	PackageExtension(ctx context.Context, options client.PackageBuildpackOptions) error
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

			if err := packager.PackageExtension(cmd.Context(), client.PackageBuildpackOptions{
				RelativeBaseDir: relativeBaseDir,
				Name:            name,
				Format:          flags.Format,
				Config:          exPackageCfg,
				Publish:         flags.Publish,
				PullPolicy:      pullPolicy,
			}); err != nil {
				return err
			}
			action := "created"
			if flags.Publish {
				action = "published"
			}

			logger.Infof("Successfully %s package %s", action, style.Symbol(name))
			return nil
		}),
	}

	// flags will be added here
	cmd.Flags().StringVarP(&flags.PackageTomlPath, "config", "c", "", "Path to package TOML config")
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "", `Format to save package as ("image" or "file")`)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish to registry (applies to "--format=image" only)`)
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")
	AddHelpFlag(cmd, "package")
	return cmd
}

func validateExtensionPackageFlags(p *ExtensionPackageFlags) error {
	if p.Publish && p.Policy == image.PullNever.String() {
		return errors.Errorf("--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
	}
	return nil
}
