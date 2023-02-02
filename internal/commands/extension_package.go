package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
)

// ExtensionPackageFlags define flags provided to the ExtensionPackage command
type ExtensionPackageFlags struct {
	PackageTomlPath   string
	Format            string
	Publish           bool
	Policy            string
	ExtensionRegistry string
	Path              string
}

// Packager and PackageConfigReader to be added here and argument also to be added in the function

// ExtensionPackage packages (a) extension(s) into OCI format, based on a package config
func ExtensionPackage(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package <name> --config <config-path>",
		Short: "Package an extension in OCI format",
		Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			// logic will be added here
			return nil
		}),
	}

	// flags will be added here

	AddHelpFlag(cmd, "package")
	return cmd
}
