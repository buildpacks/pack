package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestDelete deletes one or more manifest lists from local storage
func ManifestDelete(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [manifest-list] [manifest-list...]",
		Args:    cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
		Short:   "Remove an image from a manifest list or image index.",
		Example: `pack manifest remove cnbs/sample-package:hello-multiarch-universe`,
		Long: `Delete one or more manifest lists from local storage.
		When a manifest list exits locally, users can remove existing images from a manifest list`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return NewErrors(pack.DeleteManifest(cmd.Context(), args)).Error()
		}),
	}

	AddHelpFlag(cmd, "remove")
	return cmd
}

type Errors struct {
	errs []error
}

func NewErrors(errs []error) Errors {
	return Errors{
		errs: errs,
	}
}

func (e Errors) Error() error {
	var errMsg string
	if len(e.errs) == 0 {
		return nil
	}

	for _, err := range e.errs {
		errMsg += err.Error()
	}

	return errors.New(errMsg)
}
