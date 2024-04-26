package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestCreateFlags define flags provided to the ManifestCreate
type ManifestCreateFlags struct {
	format            string
	insecure, publish bool
}

// ManifestCreate creates an image-index/image-list for a multi-arch image
func ManifestCreate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestCreateFlags

	cmd := &cobra.Command{
		Use:   "create <manifest-list> <manifest> [<manifest> ... ] [flags]",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2), cobra.OnlyValidArgs),
		Short: "Create a manifest list or image index.",
		Example: `pack manifest create cnbs/sample-package:hello-multiarch-universe \
		cnbs/sample-package:hello-universe \
		cnbs/sample-package:hello-universe-windows`,
		Long: `Generate manifest list for a multi-arch image which will be stored locally for manipulating images within index`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			return pack.CreateManifest(
				cmd.Context(),
				client.CreateManifestOptions{
					IndexRepoName: args[0],
					RepoNames:     args[1:],
					Format:        flags.format,
					Insecure:      flags.insecure,
					Publish:       flags.publish,
				},
			)
		}),
	}

	cmdFlags := cmd.Flags()

	cmdFlags.StringVarP(&flags.format, "format", "f", "v2s2", "Format to save image index as ('OCI' or 'V2S2')")
	cmdFlags.BoolVar(&flags.insecure, "insecure", false, "Allow publishing to insecure registry")
	cmdFlags.BoolVar(&flags.publish, "publish", false, "Publish to registry")

	AddHelpFlag(cmd, "create")
	return cmd
}
