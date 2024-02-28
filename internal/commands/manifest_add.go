package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestAddFlags define flags provided to the ManifestAdd
type ManifestAddFlags struct {
	os, osVersion, osArch, osVariant  string
	osFeatures, annotations, features string
	all                               bool
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
			var (
				annotations = make(map[string]string, 0)
				features    = make([]string, 0)
				osFeatures  = make([]string, 0)
			)
			imageIndex := args[0]
			manifests := args[1]
			if err := validateManifestAddFlags(flags); err != nil {
				return err
			}

			if flags.features != "" {
				features = strings.Split(flags.features, ";")
			}

			if flags.osFeatures != "" {
				features = strings.Split(flags.osFeatures, ";")
			}

			if flags.annotations != "" {
				annotations, err = StringToKeyValueMap(flags.annotations)
				if err != nil {
					return err
				}
			}

			return pack.AddManifest(cmd.Context(), imageIndex, manifests, client.ManifestAddOptions{
				OS:          flags.os,
				OSVersion:   flags.osVersion,
				OSArch:      flags.osArch,
				OSVariant:   flags.osVariant,
				OSFeatures:  osFeatures,
				Features:    features,
				Annotations: annotations,
				All:         flags.all,
			})
		}),
	}

	cmd.Flags().BoolVar(&flags.all, "all", false, "add all of the contents to the local list (applies only if <manifest> is an index)")
	cmd.Flags().StringVar(&flags.os, "os", "", "Set the operating system")
	cmd.Flags().StringVar(&flags.osArch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.osVariant, "variant", "", "Set the architecture variant")
	cmd.Flags().StringVar(&flags.osVersion, "os-version", "", "Set the os-version")
	cmd.Flags().StringVar(&flags.osFeatures, "os-features", "", "Set the OSFeatures")
	cmd.Flags().StringVar(&flags.features, "features", "", "Set the Features")
	cmd.Flags().StringVar(&flags.annotations, "annotations", "", "Set the annotations")

	AddHelpFlag(cmd, "add")
	return cmd
}

func validateManifestAddFlags(flags ManifestAddFlags) error {
	if (flags.os != "" && flags.osArch == "") || (flags.os == "" && flags.osArch != "") {
		return errors.New("'os' or 'arch' is undefined")
	}
	return nil
}

func StringToKeyValueMap(s string) (map[string]string, error) {
	keyValues := strings.Split(s, ";")

	var annosMap = make(map[string]string)
	for _, keyValue := range keyValues {
		parts := strings.SplitN(keyValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid key-value pair: %s", keyValue)
		}

		key := parts[0]
		value := parts[1]

		if key == "" || value == "" {
			return nil, fmt.Errorf("key(%s) or value(%s) is undefined", key, value)
		}

		annosMap[key] = value
	}

	return annosMap, nil
}
