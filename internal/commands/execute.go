package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
)

func ExecuteCommand(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Executes a specific phase in the buildpacks lifecycle",
		RunE:  nil,
	}

	cmd.AddCommand(ExecuteDetect(logger, cfg, client))
	AddHelpFlag(cmd, "execute")
	return cmd
}
