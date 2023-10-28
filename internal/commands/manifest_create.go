package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestCreateFlags define flags provided to the ManifestCreate
type ManifestCreateFlags struct {
	format, registry  string
	insecure, publish bool
}

// ManifestCreate creates an image-index/image-list for a multi-arch image
func ManifestCreate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestCreateFlags

	cmd := &cobra.Command{
		Use:   "pack manifest create <manifest-list> <manifest> [<manifest> ... ] [flags]",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2), cobra.OnlyValidArgs),
		Short: "manifest create generates a manifest list for a multi-arch image",
		Example: `pack manifest create cnbs/sample-package:hello-multiarch-universe \
		cnbs/sample-package:hello-universe \
		cnbs/sample-package:hello-universe-windows`,
		Long: `Create a manifest list or manifest index for the image to support muti architecture for the image, it create a new ManifestList or ManifestIndex with the given name/repoName and adds the list of Manifests to the newly created ManifestIndex or ManifestList
		
		If the <manifest-list> already exists in the registry: pack will save a local copy of the remote manifest list,
		If the <manifest-list> doestn't exist in a registry: pack will create a local representation of the manifest list that will only save on the remote registry if the user publish it`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			// manifestList := args[0]
			// manifests := args[1:]

			// if cmd.Flags().Changed("insecure") {
			// 	flags.insecure = !flags.insecure
			// }

			// if cmd.Flags().Changed("publish") {
			// 	flags.publish = !flags.publish
			// }

			// id, err := pack.CreateManifest()

			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.format, "format", "f", "", "Format to save image index as ('OCI' or 'V2S2') (default 'v2s2')")
	cmd.Flags().BoolVar(&flags.insecure, "insecure", false, "Allow publishing to insecure registry")
	cmd.Flags().BoolVar(&flags.publish, "publish", false, "Publish to registry")
	cmd.Flags().StringVarP(&flags.registry, "registry", "r", "", "Publish to registry")

	AddHelpFlag(cmd, "create")
	return cmd
}
