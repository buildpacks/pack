package commands

import (
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"

	"github.com/spf13/cobra"
)

type ManifestCreateFlags struct {
	Publish  bool
	Insecure bool
	Registry string
	Format   string
}

func ManifestCreate(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestCreateFlags
	cmd := &cobra.Command{
		Use:   "create <manifest-list> <manifest> [<manifest> ... ]",
		Short: "Creates a manifest list",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2)),
		Example: `pack manifest create create cnbs/sample-package:hello-multiarch-universe \ 
					cnbs/sample-package:hello-universe \ 
					cnbs/sample-package:hello-universe-windows`,
		Long: "manifest create generates a manifest list for a multi-arch image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateManifestCreateFlags(&flags); err != nil {
				return err
			}

			mediaType, err := validateMediaTypeFlag(flags.Format)
			if err != nil {
				return err
			}

			packHome, err := config.PackHome()
			if err != nil {
				return err
			}

			manifestDir := filepath.Join(packHome, "manifests")

			indexName := args[0]
			manifests := args[1:]
			if err := pack.CreateManifest(cmd.Context(), client.CreateManifestOptions{
				ManifestName: indexName,
				Manifests:    manifests,
				MediaType:    mediaType,
				Publish:      flags.Publish,
				Registry:     flags.Registry,
				ManifestDir:  manifestDir,
			}); err != nil {
				return err
			}

			logger.Infof("Successfully created image index %s", style.Symbol(indexName))

			return nil
		}),
	}

	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish to registry`)
	cmd.Flags().BoolVar(&flags.Insecure, "insecure", false, `Allow publishing to insecure registry`)
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "v2s2", `Format to save image index as ("OCI" or "V2S2")`)
	cmd.Flags().StringVarP(&flags.Registry, "registry", "r", "", `Registry URL to publish the image index`)

	AddHelpFlag(cmd, "create")
	return cmd
}

func validateManifestCreateFlags(p *ManifestCreateFlags) error {
	if p.Format == "" {
		return errors.Errorf("--format flag received an empty value")
	}
	return nil
}

func validateMediaTypeFlag(format string) (imgutil.MediaTypes, error) {
	var mediaType imgutil.MediaTypes

	switch format {
	case "oci":
		mediaType = imgutil.OCITypes
	case "v2s2":
		mediaType = imgutil.DockerTypes
	default:
		return imgutil.MissingTypes, errors.Errorf("unsupported media type given for --format")
	}

	return mediaType, nil
}
