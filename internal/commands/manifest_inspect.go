package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestInspectFlags define flags provided to the ManifestInspect
// type ManifestInspectFlags struct {
// }

// ManifestInspect shows the manifest information stored in local storage
func ManifestInspect(logger logging.Logger, pack PackClient) *cobra.Command {
	// var flags ManifestInspectFlags

	cmd := &cobra.Command{
		Use:     "pack manifest inspect <manifest-list> [flags]",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short:   "manifest inspect shows the manifest information stored in local storage",
		Example: `pack manifest inspect cnbs/sample-builder:multiarch`,
		Long: `manifest inspect shows the manifest information stored in local storage.

		The inspect command will help users to view how their local manifest list looks like`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return nil
		}),
	}

	AddHelpFlag(cmd, "inspect")
	return cmd
}
