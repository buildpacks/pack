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
		Use:     "exists [manifest-list]",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short:   "Check if the given manifest list exists in local storage",
		Example: `pack manifest exists cnbs/sample-package:hello-multiarch-universe`,
		Long:    `Checks if a manifest list exists in local storage`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return pack.ExistsManifest(cmd.Context(), args[0])
		}),
	}

	AddHelpFlag(cmd, "exists")
	return cmd
}
