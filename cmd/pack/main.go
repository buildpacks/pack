package main

import (
	"os"

	"github.com/buildpack/lifecycle/image"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/logging"
)

var (
	Version           = "0.0.0"
	timestamps, quiet bool
	logger            logging.Logger
	inspect           pack.BuilderInspect
	imageFactory      image.Factory
	dockerClient      docker.Client
)

func main() {
	cobra.EnableCommandSorting = false
	rootCmd := &cobra.Command{
		Use: "pack",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger = *logging.NewLogger(os.Stdout, os.Stderr, !quiet, timestamps)
			inspect = initInspect(logger)
			imageFactory = initImageFactory(logger)
			dockerClient = initDockerClient(logger)
		},
	}
	rootCmd.PersistentFlags().BoolVar(&color.NoColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().BoolVar(&timestamps, "timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Show less output")
	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(&logger, &dockerClient, &imageFactory))
	rootCmd.AddCommand(commands.Run(&logger, &dockerClient, &imageFactory))
	rootCmd.AddCommand(commands.Rebase(&logger, &imageFactory))

	rootCmd.AddCommand(commands.CreateBuilder(&logger, &imageFactory))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(&logger))
	rootCmd.AddCommand(commands.InspectBuilder(&logger, &inspect, &imageFactory))
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

func initInspect(logger logging.Logger) pack.BuilderInspect {
	inspect, err := pack.DefaultBuilderInspect()
	if err != nil {
		exitError(logger, err)
	}
	return *inspect
}

func initImageFactory(logger logging.Logger) image.Factory {
	factory, err := image.NewFactory(image.WithOutWriter(os.Stdout))
	if err != nil {
		exitError(logger, err)
	}
	return *factory
}

func initDockerClient(logger logging.Logger) docker.Client {
	client, err := docker.New()
	if err != nil {
		exitError(logger, err)
	}
	return *client
}

func exitError(logger logging.Logger, err error) {
	logger.Error(err.Error())
	os.Exit(1)
}
