package commands

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func AddBuildpackRegistry(logger logging.Logger, cfg config.Config) *cobra.Command {
	var setDefault bool

	cmd := &cobra.Command{
		Use:   "add-buildpack-registry <name> <url> <type> (github | git)",
		Args:  cobra.ExactArgs(3),
		Short: "Adds a new buildpack registry to your pack config file",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			newRegistry := config.Registry{
				Name: args[0],
				URL:  args[1],
				Type: args[2],
			}

			err := addRegistry(newRegistry, cfg, setDefault)
			if err != nil {
				return errors.Wrapf(err, "add buildpack registry")
			}
			logger.Infof("Successfully added %s to buildpack registries", style.Symbol(newRegistry.Name))

			return nil
		}),
	}
	cmd.Example = "pack add-buildpack-registry myregistry github https://github.com/buildpacks/mybuildpack"
	cmd.Flags().BoolVar(&setDefault, "default", false, "Set this buildpack registry as the default")
	AddHelpFlag(cmd, "add-registry")

	return cmd
}

func addRegistry(newRegistry config.Registry, cfg config.Config, setDefault bool) error {
	if newRegistry.Type != "github" && newRegistry.Type != "git" {
		return errors.Errorf(
			"%s is not a valid type.  Supported types are %s or %s.",
			style.Symbol(newRegistry.Type),
			style.Symbol("github"),
			style.Symbol("git"))
	}
	for _, r := range cfg.Registries {
		if r.Name == newRegistry.Name {
			return errors.Errorf(
				"Buildpack registry %s already exists.  First run %s and try again.",
				style.Symbol(newRegistry.Name),
				style.Symbol(fmt.Sprintf("remove-buildpack-registry %s", newRegistry.Name)))
		}
	}

	if setDefault {
		cfg.DefaultRegistryName = newRegistry.Name
	}
	cfg.Registries = append(cfg.Registries, newRegistry)
	configPath, err := config.DefaultConfigPath()
	if err != nil {
		return err
	}

	return config.Write(cfg, configPath)
}
