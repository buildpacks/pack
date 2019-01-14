package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func ConfigureBuilder(logger *logging.Logger) *cobra.Command {
	var runImages []string

	cmd := &cobra.Command{
		Use:   "configure-builder <builder-image-name> --run-image <run-image-name>",
		Short: "Override a builder's default run images with one or more overrides",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}

			builder := args[0]
			cfg.ConfigureBuilder(builder, runImages)
			logger.Info("Builder %s configured", style.Symbol(builder))
			return nil
		}),
	}
	cmd.Flags().StringSliceVarP(&runImages, "run-image", "r", nil, "Overriding run image"+multiValueHelp("run image"))
	cmd.MarkFlagRequired("run-image")
	AddHelpFlag(cmd, "configure-builder")
	return cmd
}
