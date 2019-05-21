package commands

import (
	"github.com/buildpack/pack/logging"
	"github.com/spf13/cobra"
)

func SuggestStacks(logger *logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-stacks",
		Short: "Display list of recommended stacks",
		Args:  cobra.NoArgs,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			suggestStacks(logger)
			return nil
		}),
	}

	AddHelpFlag(cmd, "suggest-stacks")
	return cmd
}
