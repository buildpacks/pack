package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type PullBuildpackFlags struct {
	BuildpackRegistry string
}

func PullBuildpack(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var opts pack.PullBuildpackOptions
	var flags PullBuildpackFlags

	cmd := &cobra.Command{
		Use:     "pull-buildpack <uri>",
		Args:    cobra.ExactArgs(1),
		Short:   prependExperimental("Pull the buildpack and store it locally"),
		Example: "pack pull-buildpack my-buildpack",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			registry, err := config.GetRegistry(cfg, flags.BuildpackRegistry)
			if err != nil {
				return err
			}
			opts.URI = args[0]
			opts.RegistryType = registry.Type
			opts.RegistryURL = registry.URL
			opts.RegistryName = registry.Name

			if err := client.PullBuildpack(cmd.Context(), opts); err != nil {
				return err
			}
			logger.Infof("Successfully pulled %s", style.Symbol(opts.URI))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.BuildpackRegistry, "buildpack-registry", "r", "", "Buildpack Registry name")
	AddHelpFlag(cmd, "pull-buildpack")
	return cmd
}
