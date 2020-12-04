package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/logging"
)

func BuilderSuggest(logger logging.Logger, inspector BuilderInspector) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest",
		Args:    cobra.NoArgs,
		Short:   "Display list of recommended builders",
		Example: "pack builder suggest",
		Run: func(cmd *cobra.Command, s []string) {
			suggestBuilders(logger, inspector)
		},
	}

	AddHelpFlag(cmd, "suggest")
	return cmd
}
