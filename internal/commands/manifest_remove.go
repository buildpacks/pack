package commands

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

func ManifestDelete(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [manifest-list] [manifest-list...]",
		Short:   "Delete one or more manifest lists from local storage",
		Args:    cobra.MatchAll(cobra.MinimumNArgs(1)),
		Example: `pack manifest delete cnbs/sample-package:hello-multiarch-universe`,
		Long:    "Delete one or more manifest lists from local storage",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			indexNames := args

			packHome, err := config.PackHome()
			if err != nil {
				return err
			}

			manifestDir := filepath.Join(packHome, "manifests")

			for _, repoName := range indexNames {
				err = pack.DeleteManifest(cmd.Context(), client.DeleteManifestOptions{
					Index: repoName,
					Path:  manifestDir,
				})
				if err != nil {
					logger.Infof("Failed to remove index '%s' from local storage\n", style.Symbol(repoName))
				} else {
					logger.Infof("Successfully removed index '%s' from local storage\n", style.Symbol(repoName))
				}
			}
			return nil

		}),
	}

	AddHelpFlag(cmd, "remove")
	return cmd
}
