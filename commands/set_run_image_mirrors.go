package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func SetRunImagesMirrors(logger logging.Logger) *cobra.Command {
	var runImages []string

	cmd := &cobra.Command{
		Use:   "set-run-image-mirrors <run-image-name> --mirror <run-image-mirror>",
		Short: "Set mirrors to other repositories for a given run image",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}

			runImage := args[0]
			cfg.SetRunImageMirrors(runImage, runImages)
			for _, mirror := range runImages {
				logger.Infof("Run Image %s configured with mirror %s", style.Symbol(runImage), style.Symbol(mirror))
			}
			if len(runImages) == 0 {
				logger.Infof("All mirrors removed for Run Image %s", style.Symbol(runImage))
			}
			return nil
		}),
	}
	cmd.Flags().StringSliceVarP(&runImages, "mirror", "m", nil, "Run image mirror"+multiValueHelp("mirror"))
	AddHelpFlag(cmd, "configure-builder")
	return cmd
}
