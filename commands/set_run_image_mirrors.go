package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func SetRunImagesMirrors(logger *logging.Logger) *cobra.Command {
	var runImages []string

	cmd := &cobra.Command{
		Use:   "set-run-image-mirrors <run-image-name> --mirror <run-image-mirror>",
		Short: "Override a run images with one or more mirrors where it can download the image from",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}

			builder := args[0]
			cfg.SetRunImageMirrors(builder, runImages)
			logger.Info("Run Image %s configured with mirror '%s'", style.Symbol(builder), strings.Join(runImages, ","))
			return nil
		}),
	}
	cmd.Flags().StringSliceVarP(&runImages, "mirror", "m", nil, "Run Image mirror "+multiValueHelp("run image"))
	cmd.MarkFlagRequired("mirror")
	AddHelpFlag(cmd, "configure-builder")
	return cmd
}
