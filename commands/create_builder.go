package commands

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

type CreateBuilderFlags struct {
	BuilderTomlPath string
	Publish         bool
	NoPull          bool
}

func CreateBuilder(logger *logging.Logger, client PackClient) *cobra.Command {
	var flags CreateBuilderFlags
	ctx := createCancellableContext()
	cmd := &cobra.Command{
		Use:   "create-builder <image-name> --builder-config <builder-config-path>",
		Args:  cobra.ExactArgs(1),
		Short: "Create builder image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS == "windows" {
				return fmt.Errorf("%s is not implemented on Windows", style.Symbol("create-builder"))
			}
			builderConfig, err := readBuilderConfig(flags.BuilderTomlPath)
			if err != nil {
				return errors.Wrap(err, "invalid builder toml")
			}
			imageName := args[0]
			if err := client.CreateBuilder(ctx, pack.CreateBuilderOptions{
				BuilderName:   imageName,
				BuilderConfig: builderConfig,
				Publish:       flags.Publish,
				NoPull:        flags.NoPull,
			}); err != nil {
				return err
			}
			logger.Info("Successfully created builder image %s", style.Symbol(imageName))
			logger.Tip("Run %s to use this builder", style.Symbol(fmt.Sprintf("pack build <image-name> --builder %s", imageName)))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, "Skip pulling build image before use")
	cmd.Flags().StringVarP(&flags.BuilderTomlPath, "builder-config", "b", "", "Path to builder TOML file (required)")
	cmd.MarkFlagRequired("builder-config")
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	AddHelpFlag(cmd, "create-builder")
	return cmd
}

func readBuilderConfig(path string) (builder.Config, error) {
	builderDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return builder.Config{}, err
	}
	builderConfig := builder.Config{}
	if _, err = toml.DecodeFile(path, &builderConfig); err != nil {
		return builderConfig, fmt.Errorf(`failed to decode builder config from file %s: %s`, path, err)
	}
	for i, bp := range builderConfig.Buildpacks {
		bpURL, err := url.Parse(bp.URI)
		if err != nil {
			return builder.Config{}, err
		}
		if bpURL.Scheme == "" || bpURL.Scheme == "file" {
			if !filepath.IsAbs(bpURL.Path) {
				builderConfig.Buildpacks[i].URI = fmt.Sprintf("file://" + filepath.Join(builderDir, bpURL.Path))
			}
		}
	}
	return builderConfig, nil
}
