package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use `stack suggest` instead
func SuggestStacks(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest-stacks",
		Args:    cobra.NoArgs,
		Short:   "Display list of recommended stacks",
		Example: "pack suggest-stacks",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			deprecationWarning(logger, "suggest-stacks", "stack suggest")
			Suggest(logger)
			return nil
		}),
		Hidden: true,
	}

	AddHelpFlag(cmd, "suggest-stacks")
	return cmd
}
