package commands

import (
	"path/filepath"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/spf13/cobra"
)

func ManifestInspect(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect <manifest-list>",
		Short:   "Inspect a local manifest list",
		Args:    cobra.MatchAll(cobra.ExactArgs(1)),
		Example: `pack manifest inspect cnbs/sample-builder:multiarch`,
		Long:    "manifest inspect shows the manifest information stored in local storage",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			indexName := args[0]

			packHome, err := config.PackHome()
			if err != nil {
				return err
			}

			manifestDir := filepath.Join(packHome, "manifests")

			if err := pack.InspectManifest(cmd.Context(), client.InspectManifestOptions{
				Index: indexName,
				Path:  manifestDir,
			}); err != nil {
				return err
			}

			return nil
		}),
	}

	AddHelpFlag(cmd, "inspect")
	return cmd
}
