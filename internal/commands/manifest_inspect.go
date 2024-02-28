package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestInspect shows the manifest information stored in local storage
func ManifestInspect(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect <manifest-list>",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short:   "Display a manifest list or image index.",
		Example: `pack manifest inspect cnbs/sample-builder:multiarch`,
		Long: `manifest inspect shows the manifest information stored in local storage.
		The inspect command will help users to view how their local manifest list looks like`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return pack.InspectManifest(cmd.Context(), args[0])
		}),
	}

	AddHelpFlag(cmd, "inspect")
	return cmd
}
