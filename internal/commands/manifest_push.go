package commands

import (
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/spf13/cobra"
)

// ManifestPushFlags define flags provided to the ManifestPush
type ManifestPushFlags struct {
	format string
	insecure, purge bool
}

// ManifestPush pushes a manifest list (Image index) to a registry.
func ManifestPush(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestPushFlags

	cmd := &cobra.Command{
		Use: "pack manifest push [OPTIONS] <manifest-list> [flags]",
		Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short: "manifest push pushes a manifest list (Image index) to a registry.",
		Example: `pack manifest push cnbs/sample-package:hello-multiarch-universe`,
		Long: `manifest push pushes a manifest list (Image index) to a registry.

		Once a manifest list is ready to be published into the registry, the push command can be used`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.format, "format", "f", "", "Format to save image index as ('OCI' or 'V2S2') (default 'v2s2')")
	cmd.Flags().BoolVar(&flags.insecure, "insecure", false, "Allow publishing to insecure registry")
	cmd.Flags().BoolVar(&flags.purge, "purge", false, "Delete the manifest list or image index from local storage if pushing succeeds")

	AddHelpFlag(cmd, "push")
	return cmd
}