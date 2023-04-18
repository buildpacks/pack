package commands

import (
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/spf13/cobra"
)

type ManifestPushFlags struct {
	// Manifest list type (oci or v2s2) to use when pushing the list (default is v2s2).
	Format string

	// Allow push to an insecure registry.
	Insecure bool

	//// Delete the manifest list or image index from local storage if pushing succeeds.
	Purge bool
}

func ManifestPush(logger logging.Logger, pack PackClient) *cobra.Command {
	var flags ManifestPushFlags
	cmd := &cobra.Command{
		Use:     "push [OPTIONS] <manifest-list>",
		Short:   "Push a manifest list to a repository",
		Args:    cobra.MatchAll(cobra.ExactArgs(2)),
		Example: `pack manifest push cnbs/sample-package:hello-multiarch-universe`,
		Long:    "manifest push pushes a manifest list (Image index) to a registry.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateManifestPushFlags(&flags); err != nil {
				return err
			}

			indexName := args[0]
			if err := pack.PushManifest(cmd.Context(), client.PushManifestOptions{
				Index: indexName,
			}); err != nil {
				return err
			}
			logger.Infof("Successfully pushed the %s image index to the repository.", style.Symbol(indexName))

			return nil

		}),
	}

	cmd.Flags().BoolVar(&flags.Insecure, "insecure", false, `Allow publishing to insecure registry`)
	cmd.Flags().BoolVarP(&flags.Purge, "purge", "p", false, `Delete the manifest list or image index from local storage if pushing succeeds`)
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "", `Manifest list type (oci or v2s2) to use when pushing the list (default is v2s2)`)

	AddHelpFlag(cmd, "push")
	return cmd
}

func validateManifestPushFlags(p *ManifestPushFlags) error {
	return nil
}
