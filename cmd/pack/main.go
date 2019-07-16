package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/config"
	clilogger "github.com/buildpack/pack/internal/logging"
	"github.com/buildpack/pack/logging"
)

var (
	Version    = "0.0.0"
	packClient pack.Client
)

func main() {
	// create logger with defaults
	logger := clilogger.NewLogWithWriters()

	cobra.EnableCommandSorting = false
	cfg, err := initConfig()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

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

			packClient = initClient(logger)
		},
	}
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Show less output")
	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.Run(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.Rebase(logger, cfg, &packClient))

	rootCmd.AddCommand(commands.CreateBuilder(logger, &packClient))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(logger, cfg))
	rootCmd.AddCommand(commands.InspectBuilder(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.SetDefaultBuilder(logger, cfg, &packClient))
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

func initConfig() (config.Config, error) {
	path, err := config.DefaultConfigPath()
	if err != nil {
		return config.Config{}, errors.Wrap(err, "getting config path")
	}

	cfg, err := config.Read(path)
	if err != nil {
		return config.Config{}, errors.Wrap(err, "reading pack config")
	}
	return cfg, nil
}

func initClient(logger logging.Logger) pack.Client {
	client, err := pack.NewClient(pack.WithLogger(logger))
	if err != nil {
		exitError(logger, err)
	}
	return *client
}

func exitError(logger logging.Logger, err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
