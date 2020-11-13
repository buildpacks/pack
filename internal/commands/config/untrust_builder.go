package config

import (
	"github.com/buildpacks/pack/internal/commands"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use `trusted-builder remove` instead
func UntrustBuilder(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "untrust-builder <builder-name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Stop trusting builder",
		Long:    "Stop trusting builder.\n\nWhen building with this builder, all lifecycle phases will be no longer be run in a single container using the builder image.",
		Example: "pack untrust-builder cnbs/sample-stack-run:bionic",
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			commands.DeprecationWarning(logger, "untrust-builder", "config trusted-builder remove")
			configPath, err := config.DefaultConfigPath()
			if err != nil {
				return errors.Wrap(err, "getting config path")
			}
			return removeTrustedBuilder(args, logger, cfg, configPath)
		}),
	}

	commands.AddHelpFlag(cmd, "untrust-builder")
	return cmd
}
