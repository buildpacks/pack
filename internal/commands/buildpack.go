package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func NewBuildpackCommand(logger logging.Logger, cfg config.Config, client PackClient, packageConfigReader PackageConfigReader) *cobra.Command {
	cmd := cobra.Command{
		Use:   "buildpack",
		Short: "Interact with buildpacks",
		RunE:  nil,
	}

	cmd.AddCommand(BuildpackPackage(logger, client, packageConfigReader))
	cmd.AddCommand(BuildpackRegister(logger, cfg, client))
	cmd.AddCommand(BuildpackYank(logger, cfg, client))
	return &cmd
}
