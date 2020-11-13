package config

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use `trusted-builder add` instead
func TrustBuilder(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "trust-builder <builder-name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Trust builder",
		Long:    "Trust builder.\n\nWhen building with this builder, all lifecycle phases will be run in a single container using the builder image.",
		Example: "pack trust-builder cnbs/sample-stack-run:bionic",
		Hidden:  true,
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			commands.DeprecationWarning(logger, "trust-builder", "config trusted-builder add")
			configPath, err := config.DefaultConfigPath()
			if err != nil {
				return errors.Wrap(err, "getting config path")
			}

			return addTrustedBuilder(args, logger, cfg, configPath)
		}),
	}

	commands.AddHelpFlag(cmd, "trust-builder")
	return cmd
}
