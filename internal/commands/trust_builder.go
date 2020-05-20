package commands

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func TrustBuilder(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trust-builder <builder-name>",
		Short: "Trust builder",
		Long:  "Trust builder.\n\nWhen building with this builder, all lifecycle phases will be run in a single container using the builder image.",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || args[0] == "" {
				logger.Infof("Usage:\n\t%s\n", cmd.UseLine())
				return nil
			}

			imageName := args[0]
			builderToTrust := config.TrustedBuilder{Name: imageName}

			for _, builder := range cfg.TrustedBuilders {
				if builder == builderToTrust {
					logger.Infof("Builder %s is already trusted", style.Symbol(imageName))
					return nil
				}
			}

			cfg.TrustedBuilders = append(cfg.TrustedBuilders, builderToTrust)
			configPath, err := config.DefaultConfigPath()
			if err != nil {
				return errors.Wrap(err, "getting config path")
			}
			if err := config.Write(cfg, configPath); err != nil {
				return err
			}
			logger.Infof("Builder %s is now trusted", style.Symbol(imageName))

			return nil
		}),
	}

	AddHelpFlag(cmd, "trust-builder")
	return cmd
}
