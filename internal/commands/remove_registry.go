package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func RemoveRegistry(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	var (
		registryName string
	)

	cmd := &cobra.Command{
		Use:     "remove-registry <name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Remove registry",
		Example: "pack remove-registry myregistry",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			registryName = args[0]

			if registryName == config.OfficialRegistryName {
				return errors.Errorf("%s is a reserved registry name, please provide a different registry",
					style.Symbol(config.OfficialRegistryName))
			}

			index := findRegistryIndex(registryName, cfg.Registries)
			if index < 0 {
				return errors.Errorf("registry %s does not exist", style.Symbol(registryName))
			}

			updatedRegistries := removeRegistry(index, cfg.Registries)
			cfg.Registries = updatedRegistries

			if cfg.DefaultRegistryName == registryName {
				cfg.DefaultRegistryName = config.OfficialRegistryName
			}
			config.Write(cfg, cfgPath)

			logger.Infof("Successfully removed %s from registries", style.Symbol(registryName))

			return nil
		}),
	}

	AddHelpFlag(cmd, "remove-registry")

	return cmd
}

func findRegistryIndex(registryName string, registries []config.Registry) int {
	for index, r := range registries {
		if r.Name == registryName {
			return index
		}
	}

	return -1
}

func removeRegistry(index int, registries []config.Registry) []config.Registry {
	registries[index] = registries[len(registries)-1]

	return registries[:len(registries)-1]
}
