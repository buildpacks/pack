package commands

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func SetDefaultBuilder(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-default-builder <builder-name>",
		Args:    cobra.MaximumNArgs(1),
		Short:   "Set default builder used by other commands",
		Long:    "Set default builder used by other commands.\n\n** For suggested builders simply leave builder name empty. **",
		Example: "pack set-default-builder cnbs/sample-builder:bionic",
		RunE: LogError(logger, func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || args[0] == "" {
				logger.Infof("Usage:\n\t%s\n", cmd.UseLine())
				SuggestBuilders(logger, client)
				return nil
			}

			imageName := args[0]

			logger.Debug("Verifying local image...")
			info, err := client.InspectBuilder(imageName, true)
			if err != nil {
				return err
			}

			if info == nil {
				logger.Debug("Verifying remote image...")
				info, err := client.InspectBuilder(imageName, false)
				if err != nil {
					return err
				}

				if info == nil {
					return fmt.Errorf("builder %s not found", style.Symbol(imageName))
				}
			}

			cfg.DefaultBuilder = imageName
			configPath, err := config.DefaultConfigPath()
			if err != nil {
				return errors.Wrap(err, "getting config path")
			}
			if err := config.Write(cfg, configPath); err != nil {
				return err
			}
			logger.Infof("Builder %s is now the default builder", style.Symbol(imageName))
			return nil
		}),
	}

	AddHelpFlag(cmd, "set-default-builder")
	return cmd
}
