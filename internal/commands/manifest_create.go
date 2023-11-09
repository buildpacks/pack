package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ManifestCreateFlags define flags provided to the ManifestCreate
type ManifestCreateFlags struct {
	format, registry, os, arch    string
	insecure, publish, all, amend bool
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
		Long: `Create a manifest list or image index for the image to support muti architecture for the image, it create a new ManifestList or ImageIndex with the given name and adds the list of Manifests to the newly created ImageIndex or ManifestList
		
		If the <manifest-list> already exists in the registry: pack will save a local copy of the remote manifest list,
		If the <manifest-list> doestn't exist in a registry: pack will create a local representation of the manifest list that will only save on the remote registry if the user publish it`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageIndex := args[0]
			manifests := args[1:]
			cmdFlags := cmd.Flags()

			if err := validateManifestCreateFlags(flags); err != nil {
				return err
			}

			if cmdFlags.Changed("insecure") {
				flags.insecure = !flags.insecure
			}

			if cmdFlags.Changed("publish") {
				flags.publish = !flags.publish
			}

			id, err := pack.CreateManifest(cmd.Context(), imageIndex, manifests, client.CreateManifestOptions{
				Format:   flags.format,
				Registry: flags.registry,
				Insecure: flags.insecure,
				Publish:  flags.publish,
			})

			if err != nil {
				return err
			}
			logger.Infof("Successfully created ImageIndex/ManifestList with imageID: '%s'", id)

			return nil
		}),
	}

	cmdFlags := cmd.Flags()

	cmdFlags.StringVarP(&flags.format, "format", "f", "v2s2", "Format to save image index as ('OCI' or 'V2S2') (default 'v2s2')")
	cmdFlags.StringVarP(&flags.registry, "registry", "r", "", "Publish to registry")
	cmdFlags.StringVar(&flags.os, "os", "", "If any of the specified images is a list/index, choose the one for `os`")
	if err := cmdFlags.MarkHidden("os"); err != nil {
		panic(fmt.Sprintf("error marking --os as hidden: %v", err))
	}
	cmdFlags.StringVar(&flags.arch, "arch", "", "If any of the specified images is a list/index, choose the one for `arch`")
	if err := cmdFlags.MarkHidden("arch"); err != nil {
		panic(fmt.Sprintf("error marking --arch as hidden: %v", err))
	}
	cmdFlags.BoolVar(&flags.insecure, "insecure", false, "Allow publishing to insecure registry")
	if err := cmdFlags.MarkHidden("insecure"); err != nil {
		panic(fmt.Sprintf("error marking insecure as hidden: %v", err))
	}
	cmdFlags.BoolVar(&flags.publish, "publish", false, "Publish to registry")
	cmdFlags.BoolVar(&flags.all, "all", false, "Add all of the list's images if the images to add are lists/index")
	cmdFlags.BoolVar(&flags.amend, "amend", false, "Modify an existing list/index if one with the desired name already exists")

	AddHelpFlag(cmd, "create")
	return cmd
}

func validateManifestCreateFlags(flags ManifestCreateFlags) error {
	return nil
}