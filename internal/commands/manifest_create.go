package commands

import (
	"github.com/buildpacks/imgutil/remote"
	"github.com/buildpacks/pack/pkg/logging"

	"github.com/spf13/cobra"
)

func ManifestCreate(logger logging.Logger, pack PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <id>",
		Short: "Creates a manifest list",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(2)),
		Example: `pack manifest create paketobuildpacks/builder:full-1.0.0 \ paketobuildpacks/builder:full-linux-amd64 \
				 paketobuildpacks/builder:full-linux-arm`,
		Long: "manifest create generates a manifest list for a multi-arch image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {

			idx, _ := remote.NewIndex(args[0]) // This will return an empty index

			// Add every manifest to image index
			for _, j := range args[1:] {
				idx.Add(j)
			}

			// Store layout in local storage
			idx.Save("out/index")

			return nil
		}),
	}

	// cmd.Flags().StringVarP(&flags.API, "api", "a", "0.8", "Buildpack API compatibility of the generated buildpack")

	AddHelpFlag(cmd, "create")
	return cmd
}
