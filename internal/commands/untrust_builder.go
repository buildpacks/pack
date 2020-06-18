package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func UntrustBuilder(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "untrust-builder <builder-name>",
		Short: "Stop trusting builder",
		Long:  "Stop trusting builder.\n\nWhen building with this builder, all lifecycle phases will be no longer be run in a single container using the builder image.",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || args[0] == "" {
				logger.Infof("Usage:\n\t%s\n", cmd.UseLine())
				return nil
			}

			builderName := args[0]
			existingBuilders := cfg.TrustedBuilders
			cfg.TrustedBuilders = []config.TrustedBuilder{}
			for _, builder := range existingBuilders {
				if builder.Name == builderName {
					continue
				}

				cfg.TrustedBuilders = append(cfg.TrustedBuilders, builder)
			}

			if len(existingBuilders) == len(cfg.TrustedBuilders) {
				logger.Infof("Builder %s wasn't trusted", style.Symbol(builderName))
				return nil
			}

			configPath, err := config.DefaultConfigPath()
			if err != nil {
				return errors.Wrap(err, "getting config path")
			}
			err = config.Write(cfg, configPath)
			if err != nil {
				return errors.Wrap(err, "writing config file")
			}

			logger.Infof("Builder %s is no longer trusted", style.Symbol(builderName))
			return nil
		}),
	}

	AddHelpFlag(cmd, "untrust-builder")
	return cmd
}
