package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/logging"
)

type ExtensionInspectFlags struct {
	Depth    int
	Registry string
	Verbose  bool
}

func ExtensionInspect(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var flags ExtensionInspectFlags
	cmd := &cobra.Command{
		Use:     "inspect <extension-name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Show information about an extension",
		Example: "pack extension inspect <example-extension>",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			extensionName := args[0]
			registry := flags.Registry
			if registry == "" {
				// fix registry for extension
			}

			return extensionInspect(logger, extensionName, registry, flags, cfg, client)
		}),
	}
	// flags will be added here
	AddHelpFlag(cmd, "inspect")
	return cmd
}

func extensionInspect(logger logging.Logger, extensionName, registry string, flags ExtensionInspectFlags, cfg config.Config, client PackClient) error {
	// logic to inspect extension
	return nil
}
