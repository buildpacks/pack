package main

import (
	"os"

	"github.com/docker/docker/client"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/commands"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/logging"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	Version           = "0.0.0"
	timestamps, quiet bool
	logger            logging.Logger
	cfg               config.Config
	packClient        pack.Client
	imageFetcher      image.Fetcher
	buildpackFetcher  buildpack.Fetcher
)

func main() {
	cobra.EnableCommandSorting = false
	rootCmd := &cobra.Command{
		Use: "pack",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger = *logging.NewLogger(os.Stdout, os.Stderr, !quiet, timestamps)
			cfg = initConfig(&logger)
			imageFetcher = initImageFetcher(&logger)
			buildpackFetcher = initBuildpackFetcher(&logger)
			packClient = initClient(&cfg, &logger)
		},
	}
	rootCmd.PersistentFlags().BoolVar(&color.NoColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().BoolVar(&timestamps, "timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Show less output")
	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(&logger, &imageFetcher))
	rootCmd.AddCommand(commands.Run(&logger, &imageFetcher))
	rootCmd.AddCommand(commands.Rebase(&logger, &packClient))

	rootCmd.AddCommand(commands.CreateBuilder(&logger, &imageFetcher, &buildpackFetcher))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(&logger))
	rootCmd.AddCommand(commands.InspectBuilder(&logger, &cfg, &packClient))
	rootCmd.AddCommand(commands.SetDefaultBuilder(&logger, &packClient))
	rootCmd.AddCommand(commands.SuggestBuilders(&logger))

	rootCmd.AddCommand(commands.Version(&logger, Version))

	if err := rootCmd.Execute(); err != nil {
		if commands.IsSoftError(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func initConfig(logger *logging.Logger) config.Config {
	cfg, err := config.NewDefault()
	if err != nil {
		exitError(logger, err)
	}
	return *cfg
}

func initClient(cfg *config.Config, logger *logging.Logger) pack.Client {
	client, err := pack.DefaultClient(cfg, logger)
	if err != nil {
		exitError(logger, err)
	}
	return *client
}

func initImageFetcher(logger *logging.Logger) image.Fetcher {
	dockerClient, err := dockerClient()
	if err != nil {
		exitError(logger, err)
	}

	fetcher, err := image.NewFetcher(logger, dockerClient)
	if err != nil {
		exitError(logger, err)
	}
	return *fetcher
}

// TODO: Remove once all commands use pack.Client
func initBuildpackFetcher(logger *logging.Logger) buildpack.Fetcher {
	return *buildpack.NewFetcher(logger, cfg.Path())
}

func exitError(logger *logging.Logger, err error) {
	logger.Error(err.Error())
	os.Exit(1)
}

func dockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
}
