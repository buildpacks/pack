package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/logging"
)

func SuggestStacks(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-stacks",
		Short: "Display list of recommended stacks",
		Args:  cobra.NoArgs,
		RunE: logError(nil, func(cmd *cobra.Command, args []string) error {
			suggestStacks(logger)
			return nil
		}),
	}

	AddHelpFlag(cmd, "suggest-stacks")
	return cmd
}
