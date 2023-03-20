package commands

import (
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/spf13/cobra"
)

func NewManifestCommand(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "manifest",
		Aliases: []string{"manifest"},
		Short:   "Create manifest list",
		RunE:    nil,
	}

	cmd.AddCommand(ManifestCreate(logger))

	AddHelpFlag(cmd, "manifest")
	return cmd
}
