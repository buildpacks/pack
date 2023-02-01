package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
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
				registry = cfg.DefaultRegistryName
			}

			return extensionInspect(logger, extensionName, registry, flags, cfg, client)
		}),
	}
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", -1, "Max depth to display for Detection Order.\nOmission of this flag or values < 0 will display the entire tree.")
	cmd.Flags().StringVarP(&flags.Registry, "registry", "r", "", "buildpack registry that may be searched")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "show more output")
	AddHelpFlag(cmd, "inspect")
	return cmd
}

func extensionInspect(logger logging.Logger, extensionName, registryName string, flags ExtensionInspectFlags, cfg config.Config, pack PackClient) error {
	logger.Infof("Inspecting extension: %s\n", style.Symbol(extensionName))

	inspectedExtensionsOutput, err := inspectAllExtensions(
		pack,
		flags,
		client.InspectExtensionOptions{
			ExtensionName: extensionName,
			Daemon:        true,
			Registry:      registryName,
		},
		client.InspectExtensionOptions{
			ExtensionName: extensionName,
			Daemon:        false,
			Registry:      registryName,
		})
	if err != nil {
		return fmt.Errorf("error writing extension output: %q", err)
	}

	logger.Info(inspectedExtensionsOutput)
	return nil
}
