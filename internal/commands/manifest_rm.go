package commands

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestRemove will remove the specified image manifest if it is already referenced in the index
func ManifestRemove(logger logging.Logger, pack PackClient) *cobra.Command {
	// var flags ManifestRemoveFlags

	cmd := &cobra.Command{
		Use:   "pack manifest rm [manifest-list] [manifest] [manifest...] [flags]",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2), cobra.OnlyValidArgs),
		Short: "manifest rm will remove the specified image manifest if it is already referenced in the index",
		Example: `pack manifest rm cnbs/sample-package:hello-multiarch-universe \
		cnbs/sample-package:hello-universe-windows`,
		Long: `manifest rm will remove the specified image manifest if it is already referenced in the index.
		Sometimes users can just experiment with the feature locally and they want to discard all the local information created by pack. 'rm' command just delete the local manifest list`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			var errMsg strings.Builder
			errs := pack.RemoveManifest(cmd.Context(), args[0], args[1:])
			for _, err := range errs {
				errMsg.WriteString(err.Error() + "\n")
			}

			return errors.New(errMsg.String())
		}),
	}

	AddHelpFlag(cmd, "rm")
	return cmd
}
