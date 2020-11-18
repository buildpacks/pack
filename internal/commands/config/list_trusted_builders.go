package config

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use `trusted-builders list` instead
func ListTrustedBuilders(logger logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-trusted-builders",
		Short:   "List Trusted Builders",
		Long:    "List Trusted Builders.\n\nShow the builders that are either trusted by default or have been explicitly trusted locally using `trust-builder`",
		Example: "pack list-trusted-builders",
		Hidden:  true,
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			commands.DeprecationWarning(logger, "list-trusted-builders", "config trusted-builders list")
			listTrustedBuilders(logger, cfg)
			return nil
		}),
	}

	commands.AddHelpFlag(cmd, "list-trusted-builders")
	return cmd
}
