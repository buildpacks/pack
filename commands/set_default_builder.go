package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func SetDefaultBuilder(logger *logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-default-builder <builder-name>",
		Short: "Set default builder used by other commands",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			err = cfg.SetDefaultBuilder(args[0])
			if err != nil {
				return err
			}
			logger.Info("Builder %s is now the default builder", style.Symbol(args[0]))
			return nil
		}),
	}
	AddHelpFlag(cmd, "set-default-builder")
	return cmd
}
