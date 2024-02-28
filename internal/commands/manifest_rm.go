package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestRemove will remove the specified image manifest if it is already referenced in the index
func ManifestRemove(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm [manifest-list] [manifest] [manifest...] [flags]",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2), cobra.OnlyValidArgs),
		Short: "Remove manifest list or image index from local storage.",
		Example: `pack manifest rm cnbs/sample-package:hello-multiarch-universe \
								cnbs/sample-package@sha256:42969d8175941c21ab739d3064e9cd7e93c972a0a6050602938ed501d156e452`,
		Long: `manifest rm will remove the specified image manifest if it is already referenced in the index.
		User must pass digest of the image in oder to delete it from index.
		Sometimes users can just experiment with the feature locally and they want to discard all the local information created by pack. 'rm' command just delete the local manifest list`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return NewErrors(pack.RemoveManifest(cmd.Context(), args[0], args[1:])).Error()
		}),
	}

	AddHelpFlag(cmd, "rm")
	return cmd
}
