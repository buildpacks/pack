package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
)

func Run(logger *logging.Logger, config *config.Config, packClient *pack.Client) *cobra.Command {
	var flags BuildFlags
	var ports []string
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "run",
		Args:  cobra.NoArgs,
		Short: "Build and run app image (recommended for development only)",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if config.DefaultBuilder == "" && flags.Builder == "" {
				suggestSettingBuilder(logger, packClient)
				return MakeSoftError()
			}
			env, err := parseEnv(flags.EnvFile, flags.Env)
			if err != nil {
				return err
			}
			return packClient.Run(ctx, pack.RunOptions{
				AppDir:     flags.AppDir,
				Builder:    flags.Builder,
				RunImage:   flags.RunImage,
				Env:        env,
				NoPull:     flags.NoPull,
				ClearCache: flags.ClearCache,
				Buildpacks: flags.Buildpacks,
				Ports:      ports,
			})
		}),
	}
	buildCommandFlags(cmd, &flags)
	cmd.Flags().StringSliceVar(&ports, "port", nil, "Port to publish (defaults to port(s) exposed by container)"+multiValueHelp("port"))
	AddHelpFlag(cmd, "run")
	return cmd
}
