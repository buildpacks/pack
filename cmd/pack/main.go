package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/config"
	clilogger "github.com/buildpack/pack/internal/logging"
	"github.com/buildpack/pack/logging"
)

var (
	Version    = "0.0.0"
	cfg        config.Config
	packClient pack.Client
)

func main() {
	// create logger with defaults
	logger := clilogger.NewLogWithWriters()

	cobra.EnableCommandSorting = false
	rootCmd := &cobra.Command{
		Use: "pack",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if fs := cmd.Flags(); fs != nil {
				if flag, err := fs.GetBool("no-color"); err != nil {
					color.NoColor = flag
				}
				if flag, err := fs.GetBool("quiet"); err != nil {
					logger.WantQuiet(flag)
				}
				if flag, err := fs.GetBool("timestamps"); err != nil {
					logger.WantTime(flag)
				}
			}

			cfg = initConfig(logger)
			packClient = initClient(&cfg, logger)
		},
	}
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Show less output")
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

	rootCmd.AddCommand(commands.CompletionCommand(logger))

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
	client, err := pack.DefaultClient(cfg, pack.WithLogger(logger))
	if err != nil {
		exitError(logger, err)
	}
	return *client
}

func exitError(logger logging.Logger, err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
