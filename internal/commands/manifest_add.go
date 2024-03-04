package commands

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestAddFlags define flags provided to the ManifestAdd
type ManifestAddFlags struct {
	os, osVersion, osArch, osVariant string
	osFeatures, features             []string
	annotations                      map[string]string
	all                              bool
}

// ManifestAdd modifies a manifest list (Image index) and add a new image to the list of manifests.
func ManifestAdd(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAddFlags

	cmd := &cobra.Command{
		Use:   "add [OPTIONS] <manifest-list> <manifest> [flags]",
		Args:  cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short: "Add an image to a manifest list or image index.",
		Example: `pack manifest add cnbs/sample-package:hello-multiarch-universe \
		cnbs/sample-package:hello-universe-riscv-linux`,
		Long: `manifest add modifies a manifest list (Image index) and add a new image to the list of manifests.`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) (err error) {
			imageIndex := args[0]
			manifests := args[1]
			if err := validateManifestAddFlags(flags); err != nil {
				return err
			}

			return pack.AddManifest(cmd.Context(), imageIndex, manifests, client.ManifestAddOptions{
				OS:          flags.os,
				OSVersion:   flags.osVersion,
				OSArch:      flags.osArch,
				OSVariant:   flags.osVariant,
				OSFeatures:  flags.osFeatures,
				Features:    flags.features,
				Annotations: flags.annotations,
				All:         flags.all,
			})
		}),
	}

	cmd.Flags().BoolVar(&flags.all, "all", false, "add all of the contents to the local list (applies only if <manifest> is an index)")
	cmd.Flags().StringVar(&flags.os, "os", "", "Set the operating system")
	cmd.Flags().StringVar(&flags.osArch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.osVariant, "variant", "", "Set the architecture variant")
	cmd.Flags().StringVar(&flags.osVersion, "os-version", "", "Set the os-version")
	cmd.Flags().StringSliceVar(&flags.osFeatures, "os-features", make([]string, 0), "Set the OSFeatures")
	cmd.Flags().StringSliceVar(&flags.features, "features", make([]string, 0), "Set the Features")
	cmd.Flags().StringToStringVar(&flags.annotations, "annotations", make(map[string]string, 0), "Set the annotations")

	AddHelpFlag(cmd, "add")
	return cmd
}

func validateManifestAddFlags(flags ManifestAddFlags) error {
	if (flags.os != "" && flags.osArch == "") || (flags.os == "" && flags.osArch != "") {
		return errors.New("'os' or 'arch' is undefined")
	}
	return nil
}
