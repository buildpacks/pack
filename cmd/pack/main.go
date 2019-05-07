package main

import (
	"os"

	"github.com/apex/log"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/config"
	clilogger "github.com/buildpack/pack/internal/logging"
	"github.com/buildpack/pack/logging"
	"github.com/spf13/cobra"
)

var (
	Version    = "0.0.0"
	cfg        config.Config
	packClient pack.Client
)

func main() {
	// create logger with defaults
	handler := clilogger.NewLogHandler(os.Stderr)
	logger := clilogger.NewLogWithWriter(handler)

	cobra.EnableCommandSorting = false
	rootCmd := &cobra.Command{
		Use: "pack",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if fs := cmd.Flags(); fs != nil {
				if flag, err := fs.GetBool("no-color"); err != nil {
					handler.NoColor = flag
				}
				if flag, err := fs.GetBool("quiet"); err != nil {
					if flag {
						logger.Level = log.ErrorLevel
					}
				}
				if flag, err := fs.GetBool("timestamps"); err != nil {
					handler.WantTime = flag
				}
			}

			cfg = initConfig(logger)
			packClient = initClient(&cfg, logger)
		},
	}
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().Bool("quiet", false, "Show less output")
	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(logger, &cfg, &packClient))
	rootCmd.AddCommand(commands.Run(logger, &cfg, &packClient))
	rootCmd.AddCommand(commands.Rebase(logger, &packClient))

	rootCmd.AddCommand(commands.CreateBuilder(logger, &packClient))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(logger))
	rootCmd.AddCommand(commands.InspectBuilder(logger, &cfg, &packClient))
	rootCmd.AddCommand(commands.SetDefaultBuilder(logger, &packClient))
	rootCmd.AddCommand(commands.SuggestBuilders(logger, &packClient))

	rootCmd.AddCommand(commands.SuggestStacks(logger))
	rootCmd.AddCommand(commands.Version(logger, Version))

	if err := rootCmd.Execute(); err != nil {
		if commands.IsSoftError(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func initConfig(logger logging.Logger) config.Config {
	cfg, err := config.NewDefault()
	if err != nil {
		exitError(logger, err)
	}
	return *cfg
}

func initClient(cfg *config.Config, logger logging.Logger) pack.Client {
	client, err := pack.DefaultClient(cfg, logger)
	if err != nil {
		exitError(logger, err)
	}
	return *client
}

func exitError(logger logging.Logger, err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
