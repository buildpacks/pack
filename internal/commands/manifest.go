package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

func NewManifestCommand(logger logging.Logger, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "manifest",
		Aliases: []string{"manifest"},
		Short:   "Handle manifest list",
		RunE:    nil,
	}

	cmd.AddCommand(ManifestCreate(logger, client))
	cmd.AddCommand(ManifestAnnotate(logger, client))
	cmd.AddCommand(ManifestAdd(logger, client))
	cmd.AddCommand(ManifestPush(logger, client))
	cmd.AddCommand(ManifestRemove(logger, client))
	cmd.AddCommand(ManifestDelete(logger, client))
	cmd.AddCommand(ManifestInspect(logger, client))

	AddHelpFlag(cmd, "manifest")
	return cmd
}
