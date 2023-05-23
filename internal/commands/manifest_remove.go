package commands

import (
	"path/filepath"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/spf13/cobra"
)

func ManifestRemove(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [manifest-list] [manifest-list...]",
		Short:   "Delete one or more manifest lists from local storage",
		Args:    cobra.MatchAll(cobra.ExactArgs(2)),
		Example: `pack manifest delete cnbs/sample-package:hello-multiarch-universe`,
		Long:    "Delete one or more manifest lists from local storage",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			indexName := args[0]
			manifest := args[1]

			packHome, err := config.PackHome()
			if err != nil {
				return err
			}

			manifestDir := filepath.Join(packHome, "manifests")

			if err := pack.RemoveManifest(cmd.Context(), client.RemoveManifestOptions{
				Index:    indexName,
				Path:     manifestDir,
				Manifest: manifest,
			}); err != nil {
				return err
			}
			logger.Infof("Successfully removed the image index %s", style.Symbol(indexName))

			return nil

		}),
	}

	AddHelpFlag(cmd, "remove")
	return cmd
}
