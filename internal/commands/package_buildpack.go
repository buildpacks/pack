package commands

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type PackageBuildpackFlags struct {
	PackageTomlPath string
	ImageName       string
	OutputFile      string
	Publish         bool
	NoPull          bool
}

type PackageConfigReader interface {
	Read(path string) (pubbldpkg.Config, error)
}

type BuildpackPackager interface {
	PackageBuildpack(ctx context.Context, options pack.PackageBuildpackOptions) error
}

func PackageBuildpack(logger logging.Logger, client BuildpackPackager, packageConfigReader PackageConfigReader) *cobra.Command {
	var flags PackageBuildpackFlags
	ctx := createCancellableContext()
	cmd := &cobra.Command{
		Use:   `package-buildpack (--image <image-name> | --file <output-file>) --package-config <package-config-path>`,
		Short: "Package buildpack in OCI format.",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] != "" {
				logger.Warn(`positional argument for image name is deprecated, use "--image" instead.`)
				flags.ImageName = args[0]
			}

			if flags.ImageName == "" && flags.OutputFile == "" {
				return errors.Errorf(`must provide either "--image" or "--file" flag`)
			}

			return nil
		},
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			config, err := packageConfigReader.Read(flags.PackageTomlPath)
			if err != nil {
				return errors.Wrap(err, "reading config")
			}

			if err := client.PackageBuildpack(ctx, pack.PackageBuildpackOptions{
				ImageName:  flags.ImageName,
				OutputFile: flags.OutputFile,
				Config:     config,
				Publish:    flags.Publish,
				NoPull:     flags.NoPull,
			}); err != nil {
				return err
			}

			action := "created"
			if flags.Publish {
				action = "published"
			}

			output := flags.ImageName
			if flags.OutputFile != "" {
				output = flags.OutputFile
			}

			logger.Infof("Successfully %s package %s", action, style.Symbol(output))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.PackageTomlPath, "package-config", "p", "", "Path to package TOML config (required)")
	cmd.MarkFlagRequired("package-config")
	cmd.Flags().StringVarP(&flags.ImageName, "image", "i", "", "Save package as image")
	cmd.Flags().StringVarP(&flags.OutputFile, "file", "f", "", "Save package as file")
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish to registry (applies to "--image" only)`)
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, "Skip pulling packages before use")
	AddHelpFlag(cmd, "package-buildpack")

	return cmd
}
