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
		Use:     "untrust-builder <builder-name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Stop trusting builder",
		Long:    "Stop trusting builder.\n\nWhen building with this builder, all lifecycle phases will be no longer be run in a single container using the builder image.",
		Example: "pack untrust-builder cnbs/sample-stack-run:bionic",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			builder := args[0]

			existingTrustedBuilders := cfg.TrustedBuilders
			cfg.TrustedBuilders = []config.TrustedBuilder{}
			for _, trustedBuilder := range existingTrustedBuilders {
				if trustedBuilder.Name == builder {
					continue
				}

				cfg.TrustedBuilders = append(cfg.TrustedBuilders, trustedBuilder)
			}

			// Builder is not in the trusted builder list
			if len(existingTrustedBuilders) == len(cfg.TrustedBuilders) {
				if isSuggestedBuilder(builder) {
					// Attempted to untrust a suggested builder
					return errors.Errorf("Builder %s is a suggested builder, and is trusted by default. Currently pack doesn't support making these builders untrusted", style.Symbol(builder))
				}

				logger.Infof("Builder %s wasn't trusted", style.Symbol(builder))
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

			logger.Infof("Builder %s is no longer trusted", style.Symbol(builder))
			return nil
		}),
	}

	AddHelpFlag(cmd, "untrust-builder")
	return cmd
}
