package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestDeleteFlags define flags provided to the ManifestDelete
// type ManifestDeleteFlags struct {
// }

// ManifestExists checks if a manifest list exists in local storage
func ManifestExists(logger logging.Logger, pack PackClient) *cobra.Command {
	// var flags ManifestDeleteFlags

	cmd := &cobra.Command{
		Use:     "pack manifest exists [manifest-list]",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short:   "checks if a manifest list exists in local storage",
		Example: `pack manifest exists cnbs/sample-package:hello-multiarch-universe`,
		Long:    `Checks if a manifest list exists in local storage`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := pack.ExistsManifest(cmd.Context(), args[0]); err != nil {
				return err
			}
			return nil
		}),
	}

	AddHelpFlag(cmd, "remove")
	return cmd
}