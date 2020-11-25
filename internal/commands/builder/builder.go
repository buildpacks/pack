package builder

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func NewBuilderCommand(logger logging.Logger, cfg config.Config, client commands.PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "builder",
		Aliases: []string{"builders"},
		Short:   "Interact with builders",
		RunE:    nil,
	}

	cmd.AddCommand(Create(logger, cfg, client))
	cmd.AddCommand(Suggest(logger, client))
	commands.AddHelpFlag(cmd, "builder")
	return cmd
}
