package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func ListTrustedBuilders(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-trusted-builders",
		Short: "List Trusted Builders",
		Long:  "List Trusted Builders.\n\nShow the builders that are either trusted by default or have been explicitly trusted locally using `trust-builder`",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			logger.Info("Trusted Builders:")

			for _, builder := range suggestedBuilders {
				logger.Infof("  %s", builder.Image)
			}

			for _, builder := range cfg.TrustedBuilders {
				logger.Infof("  %s", builder.Name)
			}

			return nil
		}),
	}

	AddHelpFlag(cmd, "list-trusted-builders")
	return cmd
}
