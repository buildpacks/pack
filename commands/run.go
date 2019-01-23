package commands

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/logging"
)

func Run(ctx context.Context, logger *logging.Logger, dockerClient pack.Docker) *cobra.Command {
	var runFlags pack.RunFlags
	cmd := &cobra.Command{
		Use:   "run",
		Args:  cobra.NoArgs,
		Short: "Build and run app image (recommended for development only)",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			repoName, err := pack.RepositoryName(logger, &runFlags.BuildFlags)
			if err != nil {
				return err
			}

			cacheObj, err := cache.New(repoName, dockerClient)
			if err != nil {
				return err
			}
			bf, err := pack.DefaultBuildFactory(logger, cacheObj, dockerClient)
			if err != nil {
				return err
			}
			r, err := bf.RunConfigFromFlags(&runFlags)
			if err != nil {
				return err
			}
			return r.Run(ctx)
		}),
	}

	buildCommandFlags(cmd, &runFlags.BuildFlags)
	cmd.Flags().StringSliceVar(&runFlags.Ports, "port", nil, "Port to publish (defaults to port(s) exposed by container)"+multiValueHelp("port"))
	AddHelpFlag(cmd, "run")
	return cmd
}
