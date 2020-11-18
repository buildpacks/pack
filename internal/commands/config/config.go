package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func NewConfigCommand(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Interact with pack config",
		RunE:  nil,
	}

	cmd.AddCommand(trustedBuilder(logger, cfg, cfgPath))
	return cmd
}

type editCfgFunc func(args []string, logger logging.Logger, cfg config.Config, cfgPath string) error

func generateAdd(cmdName string, logger logging.Logger, cfg config.Config, cfgPath string, addFunc editCfgFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Args:  cobra.ExactArgs(1),
		Short: fmt.Sprintf("Add a %s", cmdName),
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			return addFunc(args, logger, cfg, cfgPath)
		}),
	}

	return cmd
}

func generateRemove(cmdName string, logger logging.Logger, cfg config.Config, cfgPath string, rmFunc editCfgFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Args:  cobra.ExactArgs(1),
		Short: fmt.Sprintf("Remove a %s", cmdName),
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			return rmFunc(args, logger, cfg, cfgPath)
		}),
	}

	return cmd
}

type listFunc func(logger logging.Logger, cfg config.Config)

func generateListCmd(cmdName string, logger logging.Logger, cfg config.Config, listFunc listFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %s", cmdName),
		RunE: commands.LogError(logger, func(cmd *cobra.Command, args []string) error {
			listFunc(logger, cfg)
			return nil
		}),
	}

	return cmd
}
