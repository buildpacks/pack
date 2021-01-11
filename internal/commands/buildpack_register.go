package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type BuildpackRegisterFlags struct {
	BuildpackRegistry string
}

func BuildpackRegister(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var opts pack.RegisterBuildpackOptions
	var flags BuildpackRegisterFlags

	cmd := &cobra.Command{
		Use:     "register <image>",
		Args:    cobra.ExactArgs(1),
		Short:   prependExperimental("Register a buildpack to a registry"),
		Example: "pack register my-buildpack",
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
	AddHelpFlag(cmd, "register")
	return cmd
}
