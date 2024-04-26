package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestAnnotateFlags define flags provided to the ManifestAnnotate
type ManifestAnnotateFlags struct {
	os, arch, variant string
	annotations       map[string]string
}

// ManifestAnnotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.
func ManifestAnnotate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAnnotateFlags

	cmd := &cobra.Command{
		Use:   "annotate [OPTIONS] <manifest-list> <manifest> [flags]",
		Args:  cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short: "Add or update information about an entry in a manifest list or image index.",
		Example: `pack manifest annotate cnbs/sample-package:hello-universe-multiarch \ 
								cnbs/sample-package:hello-universe --arch amd64`,
		Long: `manifest annotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) (err error) {
			return pack.AnnotateManifest(cmd.Context(), client.ManifestAnnotateOptions{
				IndexRepoName: args[0],
				RepoName:      args[1],
				OS:            flags.os,
				OSArch:        flags.arch,
				OSVariant:     flags.variant,
				Annotations:   flags.annotations,
			})
		}),
	}

	cmd.Flags().StringVar(&flags.os, "os", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.arch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.variant, "variant", "", "Set the architecture")
	cmd.Flags().StringToStringVar(&flags.annotations, "annotations", make(map[string]string, 0), "set an `annotation` for the specified image")

	AddHelpFlag(cmd, "annotate")
	return cmd
}
