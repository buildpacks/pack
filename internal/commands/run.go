package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/project"
	"github.com/buildpacks/pack/logging"
)

func Run(logger logging.Logger, cfg config.Config, packClient *pack.Client) *cobra.Command {
	var flags BuildFlags
	var ports []string
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "run",
		Args:  cobra.NoArgs,
		Short: "Build and run app image (recommended for development only)",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if flags.Builder == "" {
				suggestSettingBuilder(logger, packClient)
				return MakeSoftError()
			}
			env, err := parseEnv(project.Descriptor{}, flags.EnvFiles, flags.Env)
			if err != nil {
				return err
			}
			return packClient.Run(ctx, pack.RunOptions{
				AppPath:    flags.AppPath,
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
	buildCommandFlags(cmd, &flags, cfg)
	cmd.Flags().StringSliceVar(&ports, "port", nil, "Port to publish (defaults to port(s) exposed by container)"+multiValueHelp("port"))
	AddHelpFlag(cmd, "run")
	return cmd
}
