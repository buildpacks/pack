package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/logging"
)

func Run(logger *logging.Logger) *cobra.Command {
	var runFlags pack.RunFlags
	cmd := &cobra.Command{
		Use:   "run",
		Args:  cobra.NoArgs,
		Short: "Build and run app image (recommended for development only)",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			bf, err := pack.DefaultBuildFactory(logger)
			if err != nil {
				return err
			}
			r, err := bf.RunConfigFromFlags(&runFlags)
			if err != nil {
				return err
			}
			return r.Run(makeStopChannelForSignals)
		}),
	}

	buildCommandFlags(cmd, &runFlags.BuildFlags)
	cmd.Flags().StringSliceVar(&runFlags.Ports, "port", nil, "Port to publish (defaults to port(s) exposed by container)"+multiValueHelp("port"))
	AddHelpFlag(cmd, "run")
	return cmd
}
