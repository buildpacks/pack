package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestCreateFlags define flags provided to the ManifestCreate
type ManifestCreateFlags struct {
	format, os, arch       string
	insecure, publish, all bool
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
			if err := validateManifestCreateFlags(flags); err != nil {
				return err
			}

			return pack.CreateManifest(
				cmd.Context(),
				args[0],
				args[1:],
				client.CreateManifestOptions{
					Format:   flags.format,
					Insecure: flags.insecure,
					Publish:  flags.publish,
					All:      flags.all,
				},
			)
		}),
	}

	cmdFlags := cmd.Flags()

	cmdFlags.StringVarP(&flags.format, "format", "f", "v2s2", "Format to save image index as ('OCI' or 'V2S2')")
	cmdFlags.StringVar(&flags.os, "os", "", "If any of the specified images is a list/index, choose the one for `os`")
	cmdFlags.StringVar(&flags.arch, "arch", "", "If any of the specified images is a list/index, choose the one for `arch`")
	cmdFlags.BoolVar(&flags.insecure, "insecure", false, "Allow publishing to insecure registry")
	cmdFlags.BoolVar(&flags.publish, "publish", false, "Publish to registry")
	cmdFlags.BoolVar(&flags.all, "all", false, "Add all of the list's images if the images to add are lists/index")

	AddHelpFlag(cmd, "create")
	return cmd
}

func validateManifestCreateFlags(flags ManifestCreateFlags) error {
	if (flags.os != "" && flags.arch == "") || (flags.os == "" && flags.arch != "") {
		return errors.New("'os' or 'arch' is undefined")
	}
	return nil
}
