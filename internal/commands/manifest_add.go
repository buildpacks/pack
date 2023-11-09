package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestAddFlags define flags provided to the ManifestAdd
type ManifestAddFlags struct {
	ManifestAnnotateFlags
	all bool
}

// ManifestAdd modifies a manifest list (Image index) and add a new image to the list of manifests.
func ManifestAdd(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAddFlags

	cmd := &cobra.Command{
		Use:   "pack manifest add [OPTIONS] <manifest-list> <manifest> [flags]",
		Args:  cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short: "manifest add modifies a manifest list (Image index) and add a new image to the list of manifests.",
		Example: `pack manifest add cnbs/sample-package:hello-multiarch-universe \
		cnbs/sample-package:hello-universe-riscv-linux`,
		Long: `manifest add modifies a manifest list (Image index) and add a new image to the list of manifests.
		
		When a manifest list exits locally, user can add a new image to the manifest list using this command`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageIndex := args[0]
			manifests := args[1]
			if err := validateManifestAddFlags(flags); err != nil {
				return err
			}

			imageID, err := pack.AddManifest(cmd.Context(), imageIndex, manifests, client.ManifestAddOptions{

				All: flags.all,
			})

			if err != nil {
				return err
			}

			logger.Infof(imageID)
			return nil
		}),
	}

	cmd.Flags().BoolVar(&flags.all, "all", false, "add all of the contents to the local list (applies only if <manifest> is an index)")
	cmd.Flags().StringVar(&flags.os, "os", "", "Set the operating system")
	cmd.Flags().StringVar(&flags.arch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.variant, "variant", "", "Set the architecture variant")

	AddHelpFlag(cmd, "add")
	return cmd
}

func validateManifestAddFlags(flags ManifestAddFlags) error {
	return nil
}
