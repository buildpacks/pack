package commands

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

// CreateBuilderFlags define flags provided to the CreateBuilder command
type CreateBuilderFlags struct {
	BuilderTomlPath string
	Publish         bool
	NoPull          bool
	Registry        string
}

// CreateBuilder creates a builder image, based on a builder config
func CreateBuilder(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var flags CreateBuilderFlags
	cmd := &cobra.Command{
		Use:   "create-builder <image-name> --config <builder-config-path>",
		Args:  cobra.ExactArgs(1),
		Short: "Create builder image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateCreateBuilderFlags(flags, cfg); err != nil {
				return err
			}

			if cmd.Flags().Changed("builder-config") {
				logger.Warn("Flag --builder-config has been deprecated, please use --config instead")
			}

			builderConfig, warns, err := builder.ReadConfig(flags.BuilderTomlPath)
			if err != nil {
				return errors.Wrap(err, "invalid builder toml")
			}
			for _, w := range warns {
				logger.Warnf("builder configuration: %s", w)
			}

			imageName := args[0]
			if err := client.CreateBuilder(cmd.Context(), pack.CreateBuilderOptions{
				BuilderName: imageName,
				Config:      builderConfig,
				Publish:     flags.Publish,
				NoPull:      flags.NoPull,
				Registry:    flags.Registry,
			}); err != nil {
				return err
			}
			logger.Infof("Successfully created builder image %s", style.Symbol(imageName))
			logging.Tip(logger, "Run %s to use this builder", style.Symbol(fmt.Sprintf("pack build <image-name> --builder %s", imageName)))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, "Skip pulling build image before use")
	cmd.Flags().StringVarP(&flags.Registry, "buildpack-registry", "R", cfg.DefaultRegistry, "Buildpack Registry URL")
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("buildpack-registry")
	}

	cmd.Flags().StringVarP(&flags.BuilderTomlPath, "builder-config", "b", "", "Path to builder TOML file (required)")
	cmd.Flags().StringVarP(&flags.BuilderTomlPath, "config", "c", "", "Path to builder TOML file (required)")

	// TODO: Mark config required and remove builder-config after release of pack v0.12: https://github.com/buildpacks/pack/issues/694
	// cmd.MarkFlagRequired("config")
	cmd.Flags().MarkHidden("builder-config")

	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	AddHelpFlag(cmd, "create-builder")
	return cmd
}

func validateCreateBuilderFlags(flags CreateBuilderFlags, cfg config.Config) error {
	if flags.Publish && flags.NoPull {
		return errors.Errorf("The --publish and --no-pull flags cannot be used together. The --publish flag requires the use of remote images.")
	}

	if flags.Registry != "" && !cfg.Experimental {
		return pack.NewExperimentError("Support for buildpack registries is currently experimental.")
	}

	if flags.BuilderTomlPath == "" {
		return errors.Errorf("Please provide a builder config path, using --config.")
	}

	return nil
}
