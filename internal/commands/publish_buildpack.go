package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func PublishBuildpack(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var opts pack.PublishBuildpackOptions

	cmd := &cobra.Command{
		Use:   "publish-buildpack <url>",
		Args:  cobra.ExactArgs(1),
		Short: "Publishes a buildpack to the Buildpack Registry",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			opts.BuildpackageURL = args[0]
			if err := client.PublishBuildpack(cmd.Context(), opts); err != nil {
				return err
			}
			logger.Infof("Successfully published %s", style.Symbol(opts.BuildpackageURL))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&opts.BuildpackRegistry, "buildpack-registry", "R", cfg.DefaultRegistry, "Buildpack Registry URL")
	AddHelpFlag(cmd, "publish-buildpack")
	return cmd
}
