package builder

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/logging"
)

func Suggest(logger logging.Logger, inspector commands.BuilderInspector) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest",
		Args:    cobra.NoArgs,
		Short:   "Display list of recommended builders",
		Example: "pack builder suggest",
		Run: func(cmd *cobra.Command, s []string) {
			commands.SuggestBuilders(logger, inspector)
		},
	}

	commands.AddHelpFlag(cmd, "suggest")
	return cmd
}
