package commands

import (
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	pubbldpkg "github.com/buildpacks/pack/buildpackage"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: use BuildpackPackage instead
// PackageBuildpack packages (a) buildpack(s) into OCI format, based on a package config
func PackageBuildpack(logger logging.Logger, cfg config.Config, client BuildpackPackager, packageConfigReader PackageConfigReader) *cobra.Command {
	var flags BuildpackPackageFlags

	cmd := &cobra.Command{
		Use:     `package-buildpack <name> --config <package-config-path>`,
		Hidden:  true,
		Args:    cobra.ExactValidArgs(1),
		Short:   "Package buildpack in OCI format.",
		Example: "pack package-buildpack my-buildpack --config ./package.toml",
		Long: "package-buildpack allows users to package (a) buildpack(s) into OCI format, which can then to be hosted in " +
			"image repositories. You can also package a number of buildpacks together, to enable easier distribution of " +
			"a set of buildpacks. Packaged buildpacks can be used as inputs to `pack build` (using the `--buildpack` flag), " +
			"and they can be included in the configs used in `pack create-builder` and `pack package-buildpack`. For more " +
			"on how to package a buildpack, see: https://buildpacks.io/docs/buildpack-author-guide/package-a-buildpack/.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			deprecationWarning(logger, "package-buildpack", "buildpack package")

			if err := validateBuildpackPackageFlags(&flags); err != nil {
				return err
			}

			var err error
			pullPolicy, err := pubcfg.ParsePullPolicy(flags.Policy)
			if err != nil {
				return errors.Wrap(err, "parsing pull policy")
			}

			cfg := pubbldpkg.DefaultConfig()
			relativeBaseDir := ""
			if flags.PackageTomlPath != "" {
				cfg, err = packageConfigReader.Read(flags.PackageTomlPath)
				if err != nil {
					return errors.Wrap(err, "reading config")
				}

				relativeBaseDir, err = filepath.Abs(filepath.Dir(flags.PackageTomlPath))
				if err != nil {
					return errors.Wrap(err, "getting absolute path for config")
				}
			}

			name := args[0]
			if err := client.PackageBuildpack(cmd.Context(), pack.PackageBuildpackOptions{
				RelativeBaseDir: relativeBaseDir,
				Name:            name,
				Format:          flags.Format,
				Config:          cfg,
				Publish:         flags.Publish,
				PullPolicy:      pullPolicy,
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
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")

	AddHelpFlag(cmd, "package-buildpack")
	return cmd
}
