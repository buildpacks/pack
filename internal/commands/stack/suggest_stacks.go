package stack

import (
	"github.com/buildpacks/pack/internal/commands"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use `suggest` instead
func SuggestStacks(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest-stacks",
		Args:    cobra.NoArgs,
		Short:   "Display list of recommended stacks",
		Example: "pack suggest-stacks",
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			commands.DeprecationWarning(logger, "suggest-stacks", "stack suggest")
			Suggest(logger)
			return nil
		}),
		Hidden: true,
	}

	commands.AddHelpFlag(cmd, "suggest-stacks")
	return cmd
}
