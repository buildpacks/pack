package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func RegisterBuildpack(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var opts pack.RegisterBuildpackOptions

	cmd := &cobra.Command{
		Use:   "register-buildpack <url>",
		Args:  cobra.ExactArgs(1),
		Short: "Register the buildpack to a registry",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			opts.BuildpackageURL = args[0]
			if err := client.RegisterBuildpack(cmd.Context(), opts); err != nil {
				return err
			}
			logger.Infof("Successfully registered %s", style.Symbol(opts.BuildpackageURL))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&opts.BuildpackRegistry, "buildpack-registry", "R", cfg.DefaultRegistry, "Buildpack Registry URL")
	AddHelpFlag(cmd, "register-buildpack")
	return cmd
}
