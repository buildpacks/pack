package commands

import (
	"fmt"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
	"github.com/spf13/cobra"
)

func SetDefaultBuilder(logger *logging.Logger, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-default-builder <builder-name>",
		Short: "Set default builder used by other commands",
		Long:  "Set default builder used by other commands.\n\n** For suggested builders simply leave builder name empty. **",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || args[0] == "" {
				logger.Info(fmt.Sprintf("Usage:\n\t%s\n", cmd.UseLine()))
				suggestBuilders(logger, client)
				return nil
			}

			imageName := args[0]

			logger.Verbose("Verifying local image...")
			info, err := client.InspectBuilder(imageName, true)
			if err != nil {
				return err
			}

			if info == nil {
				logger.Verbose("Verifying remote image...")
				info, err := client.InspectBuilder(imageName, false)
				if err != nil {
					return err
				}

				if info == nil {
					return fmt.Errorf("builder %s not found", style.Symbol(imageName))
				}
			}

			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			err = cfg.SetDefaultBuilder(args[0])
			if err != nil {
				return err
			}
			logger.Info("Builder %s is now the default builder", style.Symbol(imageName))
			return nil
		}),
	}

	AddHelpFlag(cmd, "set-default-builder")
	return cmd
}
