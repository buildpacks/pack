package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

type RegisterBuildpackFlags struct {
	BuildpackRegistry string
}

func RegisterBuildpack(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var opts pack.RegisterBuildpackOptions
	var flags RegisterBuildpackFlags

	cmd := &cobra.Command{
		Use:   "register-buildpack <image>",
		Args:  cobra.ExactArgs(1),
		Short: "Register the buildpack to a registry",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			registry, err := config.GetRegistry(cfg, flags.BuildpackRegistry)
			if err != nil {
				return err
			}
			opts.ImageName = args[0]
			opts.Type = registry.Type
			opts.URL = registry.URL
			opts.Name = registry.Name

			if err := client.RegisterBuildpack(cmd.Context(), opts); err != nil {
				return err
			}
			logger.Infof("Successfully registered %s", style.Symbol(opts.ImageName))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.BuildpackRegistry, "buildpack-registry", "r", "", "Buildpack Registry name")
	AddHelpFlag(cmd, "register-buildpack")
	return cmd
}
