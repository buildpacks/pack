package commands

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/buildpackage"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type PackageBuildpackFlags struct {
	PackageTomlPath string
	Publish         bool
	NoPull          bool
}

func PackageBuildpack(logger logging.Logger, client PackClient) *cobra.Command {
	var flags PackageBuildpackFlags
	ctx := createCancellableContext()
	cmd := &cobra.Command{
		Use:   "package-buildpack <image-name> --package-config <package-config-path>",
		Args:  cobra.ExactArgs(1),
		Short: "Create package",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			config, err := ReadPackageConfig(flags.PackageTomlPath)
			if err != nil {
				return errors.Wrap(err, "reading config")
			}

			imageName := args[0]
			if err := client.PackageBuildpack(ctx, pack.PackageBuildpackOptions{
				Name:    imageName,
				Config:  config,
				Publish: flags.Publish,
				NoPull:  flags.NoPull,
			}); err != nil {
				return err
			}
			action := "created"
			if flags.Publish {
				action = "published"
			}
			logger.Infof("Successfully %s package %s", action, style.Symbol(imageName))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.PackageTomlPath, "package-config", "p", "", "Path to package TOML config (required)")
	cmd.MarkFlagRequired("package-config")
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, "Skip pulling packages before use")
	AddHelpFlag(cmd, "package-buildpack")

	return cmd
}

func ReadPackageConfig(path string) (buildpackage.Config, error) {
	config := buildpackage.Config{}

	configDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return config, err
	}

	_, err = toml.DecodeFile(path, &config)
	if err != nil {
		return config, errors.Wrapf(err, "reading config %s", path)
	}

	absPath, err := paths.ToAbsolute(config.Buildpack.URI, configDir)
	if err != nil {
		return config, errors.Wrapf(err, "getting absolute path for %s", style.Symbol(config.Buildpack.URI))
	}
	config.Buildpack.URI = absPath

	for i := range config.Dependencies {
		uri := config.Dependencies[i].URI
		if uri != "" {
			absPath, err := paths.ToAbsolute(uri, configDir)
			if err != nil {
				return config, errors.Wrapf(err, "getting absolute path for %s", style.Symbol(uri))
			}

			config.Dependencies[i].URI = absPath
		}
	}

	return config, nil
}
