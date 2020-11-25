package stack

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use Suggest instead
func SuggestStacks(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest-stacks",
		Args:    cobra.NoArgs,
		Short:   "Display list of recommended stacks",
		Example: "pack suggest-stacks",
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			logger.Warn("Command 'pack suggest-stacks' has been deprecated, please use 'pack stack suggest' instead")
			Suggest(logger)
			return nil
		}),
		Hidden: true,
	}

	commands.AddHelpFlag(cmd, "suggest-stacks")
	return cmd
}
