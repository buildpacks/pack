package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func DeleteStack(logger *logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-stack <stack-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete stack from list of available stacks",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.DeleteStack(args[0]); err != nil {
				return err
			}
			logger.Info("Stack %s deleted", style.Symbol(args[0]))
			return nil
		}),
	}
	AddHelpFlag(cmd, "delete-stack")
	return cmd
}
