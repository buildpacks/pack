package commands

import (
	"github.com/buildpack/pack/logging"
	"github.com/spf13/cobra"
)

func SuggestBuilders(logger *logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-builders",
		Short: "Display list of builders recommended by the Cloud Native Buildpacks project",
		Args:  cobra.NoArgs,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			suggestBuilders(logger)

			return nil
		}),
	}

	AddHelpFlag(cmd, "suggest-builders")
	return cmd
}
