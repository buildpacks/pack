package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cpkg "github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

type DownloadSBOMFlags struct {
	Local          bool
	Remote         bool
	DestinationDir string
}

func DownloadSBOM(
	logger logging.Logger,
	client PackClient,
) *cobra.Command {
	var flags DownloadSBOMFlags
	cmd := &cobra.Command{
		Use:     "download-sbom <image-name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Download SBoM from specified image",
		Long:    "Download layer containing Structured Bill of Materials (SBoM) from specified image",
		Example: "pack download-sbom buildpacksio/pack",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if flags.Local && flags.Remote {
				return errors.New("expected either '--local' or '--remote', not both")
			}

			img := args[0]
			options := cpkg.DownloadSBOMOptions{
				Daemon:         !flags.Remote,
				DestinationDir: flags.DestinationDir,
			}

			return client.DownloadSBOM(img, options)
		}),
	}
	AddHelpFlag(cmd, "download-sbom")
	cmd.Flags().BoolVar(&flags.Local, "local", false, "Pull SBoM from local daemon (Default)")
	cmd.Flags().BoolVar(&flags.Remote, "remote", false, "Pull SBoM from remote registry")
	cmd.Flags().StringVar(&flags.DestinationDir, "output-dir", ".", "Path to export SBoM contents.\nIt defaults export to the current working directory.")
	return cmd
}
