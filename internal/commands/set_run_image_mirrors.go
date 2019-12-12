package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func SetRunImagesMirrors(logger logging.Logger, cfg config.Config) *cobra.Command {
	var mirrors []string

	cmd := &cobra.Command{
		Use:   "set-run-image-mirrors <run-image-name> --mirror <run-image-mirror>",
		Short: "Set mirrors to other repositories for a given run image",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			runImage := args[0]
			cfg = config.SetRunImageMirrors(cfg, runImage, mirrors)
			configPath, err := config.DefaultConfigPath()
			if err != nil {
				return errors.Wrap(err, "getting config path")
			}
			if err := config.Write(cfg, configPath); err != nil {
				return err
			}

			for _, mirror := range mirrors {
				logger.Infof("Run Image %s configured with mirror %s", style.Symbol(runImage), style.Symbol(mirror))
			}
			if len(mirrors) == 0 {
				logger.Infof("All mirrors removed for Run Image %s", style.Symbol(runImage))
			}
			return nil
		}),
	}
	cmd.Flags().StringSliceVarP(&mirrors, "mirror", "m", nil, "Run image mirror"+multiValueHelp("mirror"))
	AddHelpFlag(cmd, "configure-builder")
	return cmd
}
