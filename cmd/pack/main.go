package main

import (
	"os"

	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/pack/docker"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/logging"
)

var (
	Version           = "0.0.0"
	timestamps, quiet bool
	logger            logging.Logger
	inspect           pack.BuilderInspect
	imageFetcher      pack.ImageFetcher
)

func main() {
	cobra.EnableCommandSorting = false
	rootCmd := &cobra.Command{
		Use: "pack",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger = *logging.NewLogger(os.Stdout, os.Stderr, !quiet, timestamps)
			inspect = initInspect(logger)
			imageFetcher = initImageFetcher(logger)
		},
	}
	rootCmd.PersistentFlags().BoolVar(&color.NoColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().BoolVar(&timestamps, "timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Show less output")
	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(&logger, &imageFetcher))
	rootCmd.AddCommand(commands.Run(&logger, &imageFetcher))
	rootCmd.AddCommand(commands.Rebase(&logger, &imageFetcher))

	rootCmd.AddCommand(commands.CreateBuilder(&logger, &imageFetcher))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(&logger))
	rootCmd.AddCommand(commands.InspectBuilder(&logger, &inspect, &imageFetcher))
	rootCmd.AddCommand(commands.SetDefaultBuilder(&logger))

	rootCmd.AddCommand(commands.Version(&logger, Version))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initInspect(logger logging.Logger) pack.BuilderInspect {
	inspect, err := pack.DefaultBuilderInspect()
	if err != nil {
		exitError(logger, err)
	}
	return *inspect
}

func initImageFetcher(logger logging.Logger) pack.ImageFetcher {
	factory, err := image.NewFactory()
	if err != nil {
		exitError(logger, err)
	}

	dockerClient, err := docker.New()
	if err != nil {
		exitError(logger, err)
	}

	return pack.ImageFetcher{
		Factory: factory,
		Docker:  dockerClient,
	}
}

func exitError(logger logging.Logger, err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
