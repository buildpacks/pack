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
		Use:     "list-registries",
		Args:    cobra.NoArgs,
		Short:   PrependExperimental("List buildpack registries"),
		Example: "pack list-registries",
		RunE: LogError(logger, func(cmd *cobra.Command, args []string) error {
			for _, registry := range config.GetRegistries(cfg) {
				isDefaultRegistry := (registry.Name == cfg.DefaultRegistryName) ||
					(registry.Name == config.OfficialRegistryName && cfg.DefaultRegistryName == "")

				logger.Info(fmtRegistry(
					registry,
					isDefaultRegistry,
					logger.IsVerbose()))
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
