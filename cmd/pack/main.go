package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/logging"
)

var (
	Version           = "0.0.0"
	timestamps, quiet bool
	logger            logging.Logger
)

func main() {
	cobra.EnableCommandSorting = false
	rootCmd := &cobra.Command{
		Use: "pack",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger = *logging.NewLogger(os.Stdout, os.Stderr, !quiet, timestamps)
		},
	}
	rootCmd.PersistentFlags().BoolVar(&color.NoColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().BoolVar(&timestamps, "timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Show less output")
	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(&logger))
	rootCmd.AddCommand(commands.Run(&logger))
	rootCmd.AddCommand(commands.Rebase(&logger))

	rootCmd.AddCommand(commands.CreateBuilder(&logger))
	rootCmd.AddCommand(commands.ConfigureBuilder(&logger))
	rootCmd.AddCommand(commands.InspectBuilder(&logger))
	rootCmd.AddCommand(commands.SetDefaultBuilder(&logger))

	rootCmd.AddCommand(commands.AddStack(&logger))
	rootCmd.AddCommand(commands.UpdateStack(&logger))
	rootCmd.AddCommand(commands.DeleteStack(&logger))
	rootCmd.AddCommand(commands.ShowStacks(&logger))
	rootCmd.AddCommand(commands.SetDefaultStack(&logger))

	rootCmd.AddCommand(commands.Version(&logger, Version))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
