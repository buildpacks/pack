package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func ListRegistries(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-registries",
		Args:  cobra.NoArgs,
		Short: "Lists all buildpack registries",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			for _, registry := range cfg.Registries {
				if registry.Name == cfg.DefaultRegistryName {
					logger.Infof("* %s", registry.Name)
				} else {
					logger.Infof("  %s", registry.Name)
				}
				if logger.IsVerbose() {
					logger.Infof("    Type: %s", registry.Type)
					logger.Infof("    URL:  %s", registry.URL)
				}
			}
			return nil
		}),
	}

	return cmd
}
