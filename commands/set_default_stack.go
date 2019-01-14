package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func SetDefaultStack(logger *logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-default-stack <stack-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Set default stack used by other commands",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			err = cfg.SetDefaultStack(args[0])
			if err != nil {
				return err
			}
			logger.Info("Stack %s is now the default stack", style.Symbol(args[0]))
			return nil
		}),
	}
	AddHelpFlag(cmd, "set-default-stack")
	return cmd
}
