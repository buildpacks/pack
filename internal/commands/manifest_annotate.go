package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestAnnotateFlags define flags provided to the ManifestAnnotate
type ManifestAnnotateFlags struct {
	os, arch, variant, osVersion            string
	features, osFeatures, urls, annotations string
}

// ManifestAnnotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.
func ManifestAnnotate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAnnotateFlags

	cmd := &cobra.Command{
		Use:   "annotate [OPTIONS] <manifest-list> <manifest> [<manifest>...] [flags]",
		Args:  cobra.MatchAll(cobra.ExactArgs(2), cobra.OnlyValidArgs),
		Short: "Add or update information about an entry in a manifest list or image index.",
		Example: `pack manifest annotate cnbs/sample-package:hello-universe-multiarch \ 
								cnbs/sample-package:hello-universe --arch amd64`,
		Long: `manifest annotate modifies a manifest list (Image index) and update the platform information for an image included in the manifest list.`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) (err error) {
			var (
				annotations = make(map[string]string, 0)
				features    = make([]string, 0)
				osFeatures  = make([]string, 0)
				urls        = make([]string, 0)
			)

			if flags.features != "" {
				features = strings.Split(flags.features, ";")
			}

			if flags.osFeatures != "" {
				osFeatures = strings.Split(flags.osFeatures, ";")
			}

			if flags.urls != "" {
				urls = strings.Split(flags.urls, ";")
			}

			if flags.annotations != "" {
				annotations, err = StringToKeyValueMap(flags.annotations)
				if err != nil {
					return err
				}
			}

			return pack.AnnotateManifest(cmd.Context(), args[0], args[1], client.ManifestAnnotateOptions{
				OS:          flags.os,
				OSVersion:   flags.osVersion,
				OSArch:      flags.arch,
				OSVariant:   flags.variant,
				OSFeatures:  osFeatures,
				Features:    features,
				URLs:        urls,
				Annotations: annotations,
			})
		}),
	}

	cmd.Flags().StringVar(&flags.os, "os", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.arch, "arch", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.variant, "variant", "", "Set the architecture")
	cmd.Flags().StringVar(&flags.osVersion, "os-version", "", "override the os `version` of the specified image")
	cmd.Flags().StringVar(&flags.features, "features", "", "override the `features` of the specified image")
	cmd.Flags().StringVar(&flags.urls, "urls", "", "override the `urls` of the specified image")
	cmd.Flags().StringVar(&flags.osFeatures, "os-features", "", "override the os `features` of the specified image")
	cmd.Flags().StringVar(&flags.annotations, "annotations", "", "set an `annotation` for the specified image")

	AddHelpFlag(cmd, "annotate")
	return cmd
}
