package commands

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/target"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

// Deprecated: Use 'builder create' instead.
// CreateBuilder creates a builder image, based on a builder config
func CreateBuilder(logger logging.Logger, cfg config.Config, pack PackClient) *cobra.Command {
	var flags BuilderCreateFlags

	cmd := &cobra.Command{
		Use:     "create-builder <image-name> --config <builder-config-path>",
		Hidden:  true,
		Args:    cobra.ExactArgs(1),
		Short:   "Create builder image",
		Example: "pack create-builder my-builder:bionic --config ./builder.toml",
		Long: `A builder is an image that bundles all the bits and information on how to build your apps, such as buildpacks, an implementation of the lifecycle, and a build-time environment that pack uses when executing the lifecycle. When building an app, you can use community builders; you can see our suggestions by running

	pack builder suggest

Creating a custom builder allows you to control what buildpacks are used and what image apps are based on. For more on how to create a builder, see: https://buildpacks.io/docs/operator-guide/create-a-builder/.
`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			deprecationWarning(logger, "create-builder", "builder create")

			if err := validateCreateFlags(&flags, cfg); err != nil {
				return err
			}

			stringPolicy := flags.Policy
			if stringPolicy == "" {
				stringPolicy = cfg.PullPolicy
			}
			pullPolicy, err := image.ParsePullPolicy(stringPolicy)
			if err != nil {
				return errors.Wrapf(err, "parsing pull policy %s", flags.Policy)
			}

			targets, err := target.ParseTargets(flags.Targets, logger)
			if err != nil {
				return err
			}

			builderConfig, warnings, err := builder.ReadMultiArchConfig(flags.BuilderTomlPath, targets)
			if err != nil {
				return errors.Wrap(err, "invalid builder toml")
			}
			for _, w := range warnings {
				logger.Warnf("builder configuration: %s", w)
			}

			if hasExtensions(builderConfig.Config) {
				if !cfg.Experimental {
					return errors.New("builder config contains image extensions; support for image extensions is currently experimental")
				}
			}

			relativeBaseDir, err := filepath.Abs(filepath.Dir(flags.BuilderTomlPath))
			if err != nil {
				return errors.Wrap(err, "getting absolute path for config")
			}

			imageName := args[0]
			builderOpts := client.CreateBuilderOptions{
				RelativeBaseDir: relativeBaseDir,
				BuilderName:     imageName,
				Config:          builderConfig,
				Publish:         flags.Publish,
				Registry:        flags.Registry,
				PullPolicy:      pullPolicy,
			}

			switch multiArch, publish := builderConfig.MultiArch(), flags.Publish; {
			case multiArch && !publish:
				builderConfig.WithTargets = []dist.Target{{OS: runtime.GOOS, Arch: runtime.GOARCH}}
				if err := pack.CreateBuilder(cmd.Context(), builderOpts); err != nil {
					return err
				}
			case multiArch && publish:
				if err := pack.CreateMultiArchBuilder(cmd.Context(), builderOpts); err != nil {
					return err
				}
			default:
				if len(builderConfig.Targets()) == 0 {
					logger.Warnf("A new '--target' flag is available to set the platform for a builder, using '%s' as default", style.Symbol("---os---"))
				}

				if err := pack.CreateBuilder(cmd.Context(), builderOpts); err != nil {
					return err
				}
			}

			logger.Infof("Successfully created builder image %s", style.Symbol(imageName))
			logging.Tip(logger, "Run %s to use this builder", style.Symbol(fmt.Sprintf("pack build <image-name> --builder %s", imageName)))
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.Registry, "buildpack-registry", "R", cfg.DefaultRegistryName, "Buildpack Registry by name")
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("buildpack-registry")
	}
	cmd.Flags().StringVarP(&flags.BuilderTomlPath, "config", "c", "", "Path to builder TOML file (required)")
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish the builder directly to the container registry specified in <image-name>, instead of the daemon.")
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")
	cmd.Flags().StringSliceVarP(&flags.Targets, "target", "t", nil,
		`Targets are the platforms list to build. one can provide target platforms in format [os][/arch][/variant]:[distroname@osversion@anotherversion];[distroname@osversion]
	- Base case for two different architectures :  '--target "linux/amd64" --target "linux/arm64"'
	- case for distribution version: '--target "windows/amd64:windows-nano@10.0.19041.1415"'
	- case for different architecture with distributed versions : '--target "linux/arm/v6:ubuntu@14.04"  --target "linux/arm/v6:ubuntu@16.04"'
	`)
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("target")
	}
	return cmd
}
