package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func NewAssetCacheCommand(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "asset-cache",
		Aliases: []string{},
		Short:   "Interact with asset caches",
		RunE:    nil,
	}

	cmd.AddCommand(CreateAssetCache(logger, cfg, client))
	AddHelpFlag(cmd, "asset-cache")
	return cmd
}
