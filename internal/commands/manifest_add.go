package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestAdd modifies a manifest list (Image index) and add a new image to the list of manifests.
func ManifestAdd(logger logging.Logger, pack PackClient) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "add [OPTIONS] <manifest-list> <manifest> [flags]",
		Args:  cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short: "Add an image to a manifest list or image index.",
		Example: `pack manifest add cnbs/sample-package:hello-multiarch-universe \
			cnbs/sample-package:hello-universe-riscv-linux`,
		Long: `manifest add modifies a manifest list (Image index) and add a new image to the list of manifests.`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) (err error) {
			return pack.AddManifest(cmd.Context(), client.ManifestAddOptions{
				IndexRepoName: args[0],
				RepoName:      args[1],
			})
		}),
	}

	AddHelpFlag(cmd, "add")
	return cmd
}
