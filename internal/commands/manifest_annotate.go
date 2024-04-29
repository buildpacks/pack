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

// ManifestAnnotate modifies a manifest list and updates the platform information within the index for an image in the list.
func ManifestAnnotate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAnnotateFlags

	cmd := &cobra.Command{
		Use:     "annotate [OPTIONS] <manifest-list> <manifest> [flags]",
		Args:    cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short:   "Add or update information about an entry in a manifest list.",
		Example: `pack manifest annotate my-image-index my-image:some-arch --arch some-other-arch`,
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

	cmd.Flags().StringVar(&flags.os, "os", "", "Set the OS")
	cmd.Flags().StringVar(&flags.arch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.variant, "variant", "", "Set the architecture variant")
	cmd.Flags().StringToStringVar(&flags.annotations, "annotations", make(map[string]string, 0), "Set an `annotation` for the specified image")

	AddHelpFlag(cmd, "annotate")
	return cmd
}
