package commands

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/buildpackage"
	"github.com/buildpack/pack/internal/paths"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

type CreatePackageFlags struct {
	PackageTomlPath string
	Publish         bool
}

func CreatePackage(logger logging.Logger, client PackClient) *cobra.Command {
	var flags CreatePackageFlags
	ctx := createCancellableContext()
	cmd := &cobra.Command{
		Use:   "create-package <image-name> --package-config <package-config-path>",
		Args:  cobra.ExactArgs(1),
		Short: "Create package",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			config, err := ReadPackageConfig(flags.PackageTomlPath)
			if err != nil {
				return errors.Wrap(err, "reading config")
			}

			imageName := args[0]
			if err := client.CreatePackage(ctx, pack.CreatePackageOptions{
				Name:    imageName,
				Config:  config,
				Publish: flags.Publish,
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
	AddHelpFlag(cmd, "create-package")
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

	for i := range config.Blobs {
		uri := config.Blobs[i].URI
		absPath, err := paths.ToAbsolute(uri, configDir)
		if err != nil {
			return config, errors.Wrapf(err, "getting absolute path for %s", style.Symbol(uri))
		}

		config.Blobs[i].URI = absPath
	}

	return config, nil
}
