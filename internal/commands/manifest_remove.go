package commands

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestDelete deletes one or more manifest lists from local storage
func ManifestDelete(logger logging.Logger, pack PackClient) *cobra.Command {
	// var flags ManifestDeleteFlags

	cmd := &cobra.Command{
		Use:     "pack manifest remove [manifest-list] [manifest-list...] [flags]",
		Args:    cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
		Short:   "Delete one or more manifest lists from local storage",
		Example: `pack manifest remove cnbs/sample-package:hello-multiarch-universe`,
		Long: `Delete one or more manifest lists from local storage.
		When a manifest list exits locally, users can remove existing images from a manifest list`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			var errMsg strings.Builder
			errs := pack.DeleteManifest(cmd.Context(), args)

			for _, err := range errs {
				errMsg.WriteString(err.Error() + "\n")
			}

			return errors.New(errMsg.String())
		}),
	}

	AddHelpFlag(cmd, "remove")
	return cmd
}
