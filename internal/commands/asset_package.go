package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func NewAssetPackageCommand(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "asset-package",
		Aliases: []string{},
		Short:   "Interact with asset packages",
		RunE:    nil,
	}

	cmd.AddCommand(CreateAssetPackage(logger, cfg, client))
	AddHelpFlag(cmd, "asset-package")
	return cmd
}
