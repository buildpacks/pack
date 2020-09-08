package cmd

import (
	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/commands/dive"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

// ConfigurableLogger defines behavior required by the PackCommand
type ConfigurableLogger interface {
	logging.Logger
	WantTime(f bool)
	WantQuiet(f bool)
	WantVerbose(f bool)
}

// NewPackCommand generates a Pack command
func NewPackCommand(logger ConfigurableLogger) (*cobra.Command, error) {
	cobra.EnableCommandSorting = false
	cfg, err := initConfig()
	if err != nil {
		return nil, err
	}

	packClient, err := initClient(logger, cfg)
	if err != nil {
		return nil, err
	}

	rootCmd := &cobra.Command{
		Use:   "pack",
		Short: "CLI for building apps using Cloud Native Buildpacks",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if fs := cmd.Flags(); fs != nil {
				if flag, err := fs.GetBool("no-color"); err == nil {
					color.Disable(flag)
				}
				if flag, err := fs.GetBool("quiet"); err == nil {
					logger.WantQuiet(flag)
				}
				if flag, err := fs.GetBool("verbose"); err == nil {
					logger.WantVerbose(flag)
				}
				if flag, err := fs.GetBool("timestamps"); err == nil {
					logger.WantTime(flag)
				}
			}
		},
	}

	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Show less output")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Show more output")
	rootCmd.Flags().Bool("version", false, "Show current 'pack' version")

	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.Rebase(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.InspectImage(logger, &cfg, &packClient))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(logger, cfg))

	rootCmd.AddCommand(commands.SetDefaultBuilder(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.InspectBuilder(logger, cfg, &packClient))
	rootCmd.AddCommand(commands.SuggestBuilders(logger, &packClient))
	rootCmd.AddCommand(commands.TrustBuilder(logger, cfg))
	rootCmd.AddCommand(commands.UntrustBuilder(logger, cfg))
	rootCmd.AddCommand(commands.ListTrustedBuilders(logger, cfg))
	rootCmd.AddCommand(commands.CreateBuilder(logger, cfg, &packClient))

	rootCmd.AddCommand(commands.PackageBuildpack(logger, &packClient, buildpackage.NewConfigReader()))

	rootCmd.AddCommand(commands.SuggestStacks(logger))

	rootCmd.AddCommand(commands.Version(logger, pack.Version))
	rootCmd.AddCommand(commands.Report(logger, pack.Version))
	rootCmd.AddCommand(dive.Dive(logger, cfg, &packClient))

	if cfg.Experimental {
		rootCmd.AddCommand(commands.RegisterBuildpack(logger, cfg, &packClient))
		rootCmd.AddCommand(commands.YankBuildpack(logger, cfg, &packClient))
	}

	rootCmd.AddCommand(commands.CompletionCommand(logger))

	rootCmd.Version = pack.Version
	rootCmd.SetVersionTemplate(`{{.Version}}{{"\n"}}`)
	rootCmd.SetOut(logging.GetWriterForLevel(logger, logging.InfoLevel))
	rootCmd.SetErr(logging.GetWriterForLevel(logger, logging.ErrorLevel))

	return rootCmd, nil
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

func initClient(logger logging.Logger, cfg config.Config) (pack.Client, error) {
	client, err := pack.NewClient(pack.WithLogger(logger), pack.WithExperimental(cfg.Experimental))
	if err != nil {
		return *client, err
	}

	return *client, nil
}
