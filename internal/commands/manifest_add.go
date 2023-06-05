package commands

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

type ManifestAddFlags struct {
	All          bool
	Architecture string
	OS           string
	Variant      string
}

func ManifestAdd(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestAddFlags
	cmd := &cobra.Command{
		Use:   "add [OPTIONS] <manifest-list> <manifest>",
		Short: "Add a new image to the manifest list",
		Args:  cobra.MatchAll(cobra.ExactArgs(2)),
		Example: `pack manifest add cnbs/sample-package:hello-multiarch-universe \ 
					cnbs/sample-package:hello-universe-riscv-linux`,
		Long: "manifest add modifies a manifest list (Image index) and add a new image to the list of manifests.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateManifestAddFlags(&flags); err != nil {
				return err
			}

			indexName := args[0]
			manifest := args[1]

			packHome, err := config.PackHome()
			if err != nil {
				return err
			}

			manifestDir := filepath.Join(packHome, "manifests")

			if err := pack.AddManifest(cmd.Context(), client.AddManifestOptions{
				Index:    indexName,
				Path:     manifestDir,
				Manifest: manifest,
				All:      flags.All,
			}); err != nil {
				return err
			}
			logger.Infof("Successfully added the image %s to the image index %s", style.Symbol(manifest), style.Symbol(indexName))

			return nil

		}),
	}

	cmd.Flags().BoolVar(&flags.All, "all", false, `add all of the contents to the local list (applies only if <manifest> is an index)`)
	cmd.Flags().StringVar(&flags.Architecture, "arch", "", "Set the architecutre")
	cmd.Flags().StringVar(&flags.OS, "os", "", "Set the operating system")
	cmd.Flags().StringVar(&flags.Variant, "variant", "", "Set the architecutre variant")

	AddHelpFlag(cmd, "add")
	return cmd
}

func validateManifestAddFlags(p *ManifestAddFlags) error {
	return nil
}
