package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/logging"
)

func SuggestBuilders(logger *logging.Logger, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-builders",
		Short: "Display list of recommended builders",
		Args:  cobra.NoArgs,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			suggestBuilders(logger, client)

			return nil
		}),
	}

	AddHelpFlag(cmd, "suggest-builders")
	return cmd
}
