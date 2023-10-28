package commands

import (
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/spf13/cobra"
)

// ManifestAnnotateFlags define flags provided to the ManifestAnnotate
type ManifestAnnotateFlags struct {
	os, arch, variant string
}

// ManifestAnnotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.
func ManifestAnnotate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAnnotateFlags

	cmd := &cobra.Command{
		Use: "pack manifest annotate [OPTIONS] <manifest-list> <manifest> [flags]",
		Args: cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short: "manifest annotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.",
		Example: `pack manifest annotate cnbs/sample-package:hello-universe-multiarch \ 
		cnbs/sample-package:hello-universe --arch amd64`,
		Long: `manifest annotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.

		Sometimes a manifest list could reference an image that doesn't specify the architecture, The "annotate" command allows users to update those values before pushing the manifest list a registry`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return nil
		}),
	}

	cmd.Flags().StringVar(&flags.os, "os", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.arch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.variant, "variant", "", "Set the architecture")

	AddHelpFlag(cmd, "annotate")
	return cmd
}