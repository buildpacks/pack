package cmd

import (
	"os"

	"github.com/heroku/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/buildpackage"
	builderwriter "github.com/buildpacks/pack/internal/builder/writer"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	imagewriter "github.com/buildpacks/pack/internal/inspectimage/writer"
	"github.com/buildpacks/pack/internal/term"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

// ConfigurableLogger defines behavior required by the PackCommand
type ConfigurableLogger interface {
	logging.Logger
	WantTime(f bool)
	WantQuiet(f bool)
	WantVerbose(f bool)
}

// clientHolder defers client initialization until PersistentPreRunE,
// allowing root persistent flags
type clientHolder struct {
	commands.PackClient
}

// NewPackCommand generates a Pack command
//
//nolint:staticcheck
func NewPackCommand(logger ConfigurableLogger) (*cobra.Command, error) {
	cobra.EnableCommandSorting = false
	cfg, cfgPath, err := initConfig()
	if err != nil {
		return nil, err
	}

	holder := &clientHolder{}
	var dockerHost string

	rootCmd := &cobra.Command{
		Use:   "pack",
		Short: "CLI for building apps using Cloud Native Buildpacks",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if dockerHost != "" && dockerHost != "inherit" {
				os.Setenv("DOCKER_HOST", dockerHost)
			}

			packClient, err := initClient(logger, cfg)
			if err != nil {
				return err
			}
			holder.PackClient = packClient

			if fs := cmd.Flags(); fs != nil {
				if forceColor, err := fs.GetBool("force-color"); err == nil && !forceColor {
					if flag, err := fs.GetBool("no-color"); err == nil && flag {
						color.Disable(flag)
					}

					_, canDisplayColor := term.IsTerminal(logging.GetWriterForLevel(logger, logging.InfoLevel))
					if !canDisplayColor {
						color.Disable(true)
					}
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

			return nil
		},
	}

	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("force-color", false, "Force color output")
	rootCmd.PersistentFlags().Bool("timestamps", false, "Enable timestamps in output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Show less output")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Show more output")
	rootCmd.PersistentFlags().StringVar(&dockerHost, "docker-host", "",
		`Address to docker daemon to connect to.
If not set (or set to empty string) the standard socket location will be used.
Special value 'inherit' may be used in which case DOCKER_HOST environment variable will be used.
This flag is available on all subcommands; for 'build', it controls which daemon is
exposed to the build container's lifecycle phases.
`)
	rootCmd.Flags().Bool("version", false, "Show current 'pack' version")

	commands.AddHelpFlag(rootCmd, "pack")

	rootCmd.AddCommand(commands.Build(logger, cfg, holder))
	rootCmd.AddCommand(commands.NewBuilderCommand(logger, cfg, holder))
	rootCmd.AddCommand(commands.NewBuildpackCommand(logger, cfg, holder, buildpackage.NewConfigReader()))
	rootCmd.AddCommand(commands.NewExtensionCommand(logger, cfg, holder, buildpackage.NewConfigReader()))
	rootCmd.AddCommand(commands.NewConfigCommand(logger, cfg, cfgPath, holder))
	rootCmd.AddCommand(commands.InspectImage(logger, imagewriter.NewFactory(), cfg, holder))
	rootCmd.AddCommand(commands.NewStackCommand(logger))
	rootCmd.AddCommand(commands.Rebase(logger, cfg, holder))
	rootCmd.AddCommand(commands.NewSBOMCommand(logger, cfg, holder))

	rootCmd.AddCommand(commands.InspectBuildpack(logger, cfg, holder))
	rootCmd.AddCommand(commands.InspectBuilder(logger, cfg, holder, builderwriter.NewFactory()))

	rootCmd.AddCommand(commands.SetDefaultBuilder(logger, cfg, cfgPath, holder))
	rootCmd.AddCommand(commands.SetRunImagesMirrors(logger, cfg, cfgPath))
	rootCmd.AddCommand(commands.SuggestBuilders(logger, holder))
	rootCmd.AddCommand(commands.TrustBuilder(logger, cfg, cfgPath))
	rootCmd.AddCommand(commands.UntrustBuilder(logger, cfg, cfgPath))
	rootCmd.AddCommand(commands.ListTrustedBuilders(logger, cfg))
	rootCmd.AddCommand(commands.CreateBuilder(logger, cfg, holder))
	rootCmd.AddCommand(commands.PackageBuildpack(logger, cfg, holder, buildpackage.NewConfigReader()))

	if cfg.Experimental {
		rootCmd.AddCommand(commands.AddBuildpackRegistry(logger, cfg, cfgPath))
		rootCmd.AddCommand(commands.ListBuildpackRegistries(logger, cfg))
		rootCmd.AddCommand(commands.RegisterBuildpack(logger, cfg, holder))
		rootCmd.AddCommand(commands.SetDefaultRegistry(logger, cfg, cfgPath))
		rootCmd.AddCommand(commands.RemoveRegistry(logger, cfg, cfgPath))
		rootCmd.AddCommand(commands.YankBuildpack(logger, cfg, holder))
		rootCmd.AddCommand(commands.NewManifestCommand(logger, holder))
	}

	packHome, err := config.PackHome()
	if err != nil {
		return nil, err
	}

	rootCmd.AddCommand(commands.CompletionCommand(logger, packHome))
	rootCmd.AddCommand(commands.Report(logger, client.Version, cfgPath))
	rootCmd.AddCommand(commands.Version(logger, client.Version))

	rootCmd.Version = client.Version
	rootCmd.SetVersionTemplate(`{{.Version}}{{"\n"}}`)
	rootCmd.SetOut(logging.GetWriterForLevel(logger, logging.InfoLevel))
	rootCmd.SetErr(logging.GetWriterForLevel(logger, logging.ErrorLevel))

	return rootCmd, nil
}

func initConfig() (config.Config, string, error) {
	path, err := config.DefaultConfigPath()
	if err != nil {
		return config.Config{}, "", errors.Wrap(err, "getting config path")
	}

	cfg, err := config.Read(path)
	if err != nil {
		return config.Config{}, "", errors.Wrap(err, "reading pack config")
	}
	return cfg, path, nil
}

func initClient(logger logging.Logger, cfg config.Config) (*client.Client, error) {
	if err := client.ProcessDockerContext(logger); err != nil {
		return nil, err
	}

	dc, err := tryInitSSHDockerClient()
	if err != nil {
		return nil, err
	}

	// If we got a docker client from SSH, use it directly
	if dc != nil {
		return client.NewClient(client.WithLogger(logger), client.WithExperimental(cfg.Experimental), client.WithRegistryMirrors(cfg.RegistryMirrors), client.WithDockerClient(dc))
	}

	return client.NewClient(client.WithLogger(logger), client.WithExperimental(cfg.Experimental), client.WithRegistryMirrors(cfg.RegistryMirrors))
}
