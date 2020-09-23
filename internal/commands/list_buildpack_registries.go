package commands

import (
	"fmt"

	"github.com/buildpacks/pack/internal/style"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func ListBuildpackRegistries(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-buildpack-registries",
		Args:  cobra.NoArgs,
		Short: "Lists all buildpack registries",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			for _, registry := range cfg.Registries {
				registryFmt := fmtRegistry(registry, registry.Name == cfg.DefaultRegistryName, logger.IsVerbose())
				logger.Info(registryFmt)
			}
			logging.Tip(logger, "Run %s to add additional registries", style.Symbol("pack add-buildpack-registry"))

			return nil
		}),
	}

	return cmd
}

func fmtRegistry(registry config.Registry, isDefaultRegistry, isVerbose bool) string {
	registryOutput := fmt.Sprintf("  %s", registry.Name)
	if isDefaultRegistry {
		registryOutput = fmt.Sprintf("* %s", registry.Name)
	}
	if isVerbose {
		registryOutput = fmt.Sprintf("%-12s %s", registryOutput, registry.URL)
	}

	return registryOutput
}
