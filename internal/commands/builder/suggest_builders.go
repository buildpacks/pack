package builder

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use Suggest instead.
func SuggestBuilders(logger logging.Logger, inspector commands.BuilderInspector) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest-builders",
		Hidden:  true,
		Args:    cobra.NoArgs,
		Short:   "Display list of recommended builders",
		Example: "pack suggest-builders",
		Run: func(cmd *cobra.Command, s []string) {
			logger.Warn("Command 'pack suggest-builder' has been deprecated, please use 'pack builder suggest' instead")
			commands.SuggestBuilders(logger, inspector)
		},
	}

	return cmd
}
