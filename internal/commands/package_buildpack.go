package commands

import (
	"context"

	pubcfg "github.com/buildpacks/pack/config"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

// PackageBuildpackFlags define flags provided to the PackageBuildpack command
type PackageBuildpackFlags struct {
	PackageTomlPath string
	Format          string
	OS              string
	Publish         bool
	Policy          string
}

// BuildpackPackager packages buildpacks
type BuildpackPackager interface {
	PackageBuildpack(ctx context.Context, options pack.PackageBuildpackOptions) error
}

// PackageConfigReader reads PackageBuildpack configs
type PackageConfigReader interface {
	Read(path string) (pubbldpkg.Config, error)
}

// PackageBuildpack packages (a) buildpack(s) into OCI format, based on a package config
func PackageBuildpack(logger logging.Logger, cfg config.Config, client BuildpackPackager, packageConfigReader PackageConfigReader) *cobra.Command {
	var flags PackageBuildpackFlags

	cmd := &cobra.Command{
		Use:   `package-buildpack <name> --config <package-config-path>`,
		Short: "Package buildpack in OCI format.",
		Args:  cobra.ExactValidArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validatePackageBuildpackFlags(&flags, cfg); err != nil {
				return err
			}

			var err error
			pullPolicy, err := pubcfg.ParsePullPolicy(flags.Policy)
			if err != nil {
				return errors.Wrap(err, "parsing pull policy")
			}

			if cmd.Flags().Changed("package-config") {
				logger.Warn("Flag --package-config has been deprecated, please use --config instead")
			}

			cfg, err := packageConfigReader.Read(flags.PackageTomlPath)
			if err != nil {
				return errors.Wrap(err, "reading config")
			}

			name := args[0]
			if err := client.PackageBuildpack(cmd.Context(), pack.PackageBuildpackOptions{
				Name:       name,
				Format:     flags.Format,
				OS:         flags.OS,
				Config:     cfg,
				Publish:    flags.Publish,
				PullPolicy: pullPolicy,
			}); err != nil {
				return err
			}

			action := "created"
			if flags.Publish {
				action = "published"
			}

			logger.Infof("Successfully %s package %s", action, style.Symbol(name))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.PackageTomlPath, "config", "c", "", "Path to package TOML config (required)")

	cmd.Flags().StringVarP(&flags.Format, "format", "f", "", `Format to save package as ("image" or "file")`)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, `Publish to registry (applies to "--format=image" only)`)
	cmd.Flags().StringVar(&flags.OS, "os", "", `Operating system format of the package OCI image: "linux" or "windows" (defaults to "linux", except local images which use the daemon OS)`)
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("os")
	}
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")

	AddHelpFlag(cmd, "package-buildpack")
	return cmd
}

func validatePackageBuildpackFlags(p *PackageBuildpackFlags, cfg config.Config) error {
	if p.Publish && p.Policy == pubcfg.PullNever.String() {
		return errors.Errorf("--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
	}

	if p.PackageTomlPath == "" {
		return errors.Errorf("Please provide a package config path, using --config")
	}

	if p.OS != "" && !cfg.Experimental {
		return pack.NewExperimentError("Support for OS flag is currently experimental.")
	}

	return nil
}
