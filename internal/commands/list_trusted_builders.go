package commands

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func ListTrustedBuilders(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-trusted-builders",
		Short: "List Trusted Builders",
		Long:  "List Trusted Builders.\n\nShow the builders that are either trusted by default or have been explicitly trusted locally using `trust-builder`",
		RunE: LogError(logger, func(cmd *cobra.Command, args []string) error {
			logger.Info("Trusted Builders:")

			var trustedBuilders []string
			for _, builder := range suggestedBuilders {
				trustedBuilders = append(trustedBuilders, builder.Image)
			}

			for _, builder := range cfg.TrustedBuilders {
				trustedBuilders = append(trustedBuilders, builder.Name)
			}

			sort.Strings(trustedBuilders)

			for _, builder := range trustedBuilders {
				logger.Infof("  %s", builder)
			}

			return nil
		}),
	}

	AddHelpFlag(cmd, "list-trusted-builders")
	return cmd
}
