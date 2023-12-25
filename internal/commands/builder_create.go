package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

// BuilderCreateFlags define flags provided to the CreateBuilder command
type BuilderCreateFlags struct {
	Flatten         bool
	Publish         bool
	BuilderTomlPath string
	Registry        string
	Policy          string
	FlattenExclude  []string
}

// CreateBuilder creates a builder image, based on a builder config
func BuilderCreate(logger logging.Logger, cfg config.Config, pack PackClient) *cobra.Command {
	var flags BuilderCreateFlags

	cmd := &cobra.Command{
		Use:     "create <image-name> --config <builder-config-path>",
		Args:    cobra.ExactArgs(1),
		Short:   "Create builder image",
		Example: "pack builder create my-builder:bionic --config ./builder.toml",
		Long: `A builder is an image that bundles all the bits and information on how to build your apps, such as buildpacks, an implementation of the lifecycle, and a build-time environment that pack uses when executing the lifecycle. When building an app, you can use community builders; you can see our suggestions by running

	pack builders suggest

Creating a custom builder allows you to control what buildpacks are used and what image apps are based on. For more on how to create a builder, see: https://buildpacks.io/docs/operator-guide/create-a-builder/.
`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
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

			builderConfig, warns, err := builder.ReadConfig(flags.BuilderTomlPath)
			if err != nil {
				return errors.Wrap(err, "invalid builder toml")
			}
			for _, w := range warns {
				logger.Warnf("builder configuration: %s", w)
			}

			if hasExtensions(builderConfig) {
				if !cfg.Experimental {
					return errors.New("builder config contains image extensions; support for image extensions is currently experimental")
				}
			}

			relativeBaseDir, err := filepath.Abs(filepath.Dir(flags.BuilderTomlPath))
			if err != nil {
				return errors.Wrap(err, "getting absolute path for config")
			}

			envMap, warnings, err := builder.ParseBuildConfigEnv(builderConfig.Build.Env, flags.BuilderTomlPath)
			for _, v := range warnings {
				logger.Warn(v)
			}
			if err != nil {
				return err
			}

			imageName := args[0]
			if err := pack.CreateBuilder(cmd.Context(), client.CreateBuilderOptions{
				RelativeBaseDir: relativeBaseDir,
				BuildConfigEnv:  envMap,
				BuilderName:     imageName,
				Config:          builderConfig,
				Publish:         flags.Publish,
				Registry:        flags.Registry,
				PullPolicy:      pullPolicy,
				Flatten:         flags.Flatten,
				FlattenExclude:  flags.FlattenExclude,
			}); err != nil {
				return err
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
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	cmd.Flags().StringVar(&flags.Policy, "pull-policy", "", "Pull policy to use. Accepted values are always, never, and if-not-present. The default is always")
	cmd.Flags().BoolVar(&flags.Flatten, "flatten", false, "Flatten each composite buildpack into a single layer")
	cmd.Flags().StringSliceVarP(&flags.FlattenExclude, "flatten-exclude", "e", nil, "Buildpacks to exclude from flattening, in the form of '<buildpack-id>@<buildpack-version>'")

	AddHelpFlag(cmd, "create")
	return cmd
}

func hasExtensions(builderConfig builder.Config) bool {
	return len(builderConfig.Extensions) > 0 || len(builderConfig.OrderExtensions) > 0
}

func validateCreateFlags(flags *BuilderCreateFlags, cfg config.Config) error {
	if flags.Publish && flags.Policy == image.PullNever.String() {
		return errors.Errorf("--publish and --pull-policy never cannot be used together. The --publish flag requires the use of remote images.")
	}

	if flags.Registry != "" && !cfg.Experimental {
		return client.NewExperimentError("Support for buildpack registries is currently experimental.")
	}

	if flags.BuilderTomlPath == "" {
		return errors.Errorf("Please provide a builder config path, using --config.")
	}

	if flags.Flatten && len(flags.FlattenExclude) > 0 {
		for _, exclude := range flags.FlattenExclude {
			if strings.Count(exclude, "@") != 1 {
				return errors.Errorf("invalid format %s; please use '<buildpack-id>@<buildpack-version>' to exclude buildpack from flattening", exclude)
			}
		}
	}

	return nil
}
