package commands

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

func ManifestRemove(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm [manifest-list] [manifest]",
		Short: "Remove an image manifest from index",
		Args:  cobra.MatchAll(cobra.ExactArgs(2)),
		Example: `pack manifest rm cnbs/sample-package:hello-multiarch-universe \
					cnbs/sample-package:hello-universe-windows`,
		Long: "manifest remove will remove the specified image manifest if it is already referenced in the index",
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
			logger.Infof("Successfully removed the manifest '%s' from image index %s", style.Symbol(manifest), style.Symbol(indexName))

			return nil

		}),
	}

	AddHelpFlag(cmd, "rm")
	return cmd
}
