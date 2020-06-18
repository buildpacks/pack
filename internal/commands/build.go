package commands

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	ignore "github.com/sabhiram/go-gitignore"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/project"
)

type BuildFlags struct {
	AppPath            string
	Builder            string
	Registry           string
	RunImage           string
	Env                []string
	EnvFiles           []string
	Publish            bool
	NoPull             bool
	ClearCache         bool
	TrustBuilder       bool
	Buildpacks         []string
	Network            string
	DescriptorPath     string
	Volumes            []string
	DefaultProcessType string
}

// Build an image from source code
func Build(logger logging.Logger, cfg config.Config, packClient PackClient) *cobra.Command {
	var flags BuildFlags

	cmd := &cobra.Command{
		Use:   "build <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Generate app image from source code",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateBuildFlags(flags, logger, cfg, packClient); err != nil {
				return err
			}

			imageName := args[0]

			descriptor, actualDescriptorPath, err := parseProjectToml(flags.AppPath, flags.DescriptorPath)
			if err != nil {
				return err
			}
			if actualDescriptorPath != "" {
				logger.Debugf("Using project descriptor located at %s", style.Symbol(actualDescriptorPath))
			}

			fileFilter, err := getFileFilter(descriptor)
			if err != nil {
				return err
			}

			env, err := parseEnv(descriptor, flags.EnvFiles, flags.Env)
			if err != nil {
				return err
			}

			buildpacks := flags.Buildpacks
			if len(buildpacks) == 0 {
				buildpacks = []string{}
				projectDescriptorDir := filepath.Dir(actualDescriptorPath)
				for _, bp := range descriptor.Build.Buildpacks {
					if len(bp.URI) == 0 {
						// there are several places through out the pack code where the "id@version" format is used.
						// we should probably central this, but it's not clear where it belongs
						buildpacks = append(buildpacks, fmt.Sprintf("%s@%s", bp.ID, bp.Version))
					} else {
						uri, err := paths.ToAbsolute(bp.URI, projectDescriptorDir)
						if err != nil {
							return err
						}
						buildpacks = append(buildpacks, uri)
					}
				}
			}

			var cfgTrustedBuilder bool
			for _, trustedBuilder := range cfg.TrustedBuilders {
				logger.Debugf("Builder %s is trusted", style.Symbol(trustedBuilder.Name))
				if flags.Builder == trustedBuilder.Name {
					cfgTrustedBuilder = true
					break
				}
			}

			var suggestedBuilder bool
			for _, builder := range suggestedBuilders {
				if flags.Builder == builder.Image {
					suggestedBuilder = true
					break
				}
			}

			if err := packClient.Build(cmd.Context(), pack.BuildOptions{
				AppPath:           flags.AppPath,
				Builder:           flags.Builder,
				Registry:          flags.Registry,
				AdditionalMirrors: getMirrors(cfg),
				RunImage:          flags.RunImage,
				Env:               env,
				Image:             imageName,
				Publish:           flags.Publish,
				NoPull:            flags.NoPull,
				ClearCache:        flags.ClearCache,
				TrustBuilder:      flags.TrustBuilder || cfgTrustedBuilder || suggestedBuilder,
				Buildpacks:        buildpacks,
				ContainerConfig: pack.ContainerConfig{
					Network: flags.Network,
					Volumes: flags.Volumes,
				},
				DefaultProcessType: flags.DefaultProcessType,
				FileFilter:         fileFilter,
			}); err != nil {
				return errors.Wrap(err, "failed to build")
			}
			logger.Infof("Successfully built image %s", style.Symbol(imageName))
			return nil
		}),
	}
	buildCommandFlags(cmd, &flags, cfg)
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	AddHelpFlag(cmd, "build")
	return cmd
}

func buildCommandFlags(cmd *cobra.Command, buildFlags *BuildFlags, cfg config.Config) {
	cmd.Flags().StringVarP(&buildFlags.AppPath, "path", "p", "", "Path to app dir or zip-formatted file (defaults to current working directory)")
	cmd.Flags().StringVarP(&buildFlags.Builder, "builder", "B", cfg.DefaultBuilder, "Builder image")
	cmd.Flags().StringVarP(&buildFlags.Registry, "buildpack-registry", "R", cfg.DefaultRegistry, "Buildpack Registry URL")
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("buildpack-registry")
	}
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringArrayVarP(&buildFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file.")
	cmd.Flags().StringArrayVar(&buildFlags.EnvFiles, "env-file", []string{}, "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed")
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "Skip pulling builder and run images before use")
	cmd.Flags().BoolVar(&buildFlags.ClearCache, "clear-cache", false, "Clear image's associated cache before building")
	cmd.Flags().BoolVar(&buildFlags.TrustBuilder, "trust-builder", false, "Trust the provided builder\nAll lifecycle phases will be run in a single container (if supported by the lifecycle).")
	cmd.Flags().StringSliceVarP(&buildFlags.Buildpacks, "buildpack", "b", nil, "Buildpack reference in the form of '<buildpack>@<version>',\n  path to a buildpack directory (not supported on Windows),\n  path/URL to a buildpack .tar or .tgz file, or\n  the name of a packaged buildpack image"+multiValueHelp("buildpack"))
	cmd.Flags().StringVar(&buildFlags.Network, "network", "", "Connect detect and build containers to network")
	cmd.Flags().StringVarP(&buildFlags.DescriptorPath, "descriptor", "d", "", "Path to the project descriptor file")
	cmd.Flags().StringArrayVar(&buildFlags.Volumes, "volume", nil, "Mount host volume into the build container, in the form '<host path>:<target path>'. Target path will be prefixed with '/platform/'"+multiValueHelp("volume"))
	cmd.Flags().StringVarP(&buildFlags.DefaultProcessType, "default-process", "D", "", "Set the default process type")
}

func validateBuildFlags(flags BuildFlags, logger logging.Logger, cfg config.Config, packClient PackClient) error {
	if flags.Builder == "" {
		suggestSettingBuilder(logger, packClient)
		return pack.NewSoftError()
	}

	if flags.Registry != "" && !cfg.Experimental {
		return pack.NewExperimentError("Support for buildpack registries is currently experimental.")
	}

	return nil
}

func parseEnv(project project.Descriptor, envFiles []string, envVars []string) (map[string]string, error) {
	env := map[string]string{}

	for _, envVar := range project.Build.Env {
		env[envVar.Name] = envVar.Value
	}
	for _, envFile := range envFiles {
		envFileVars, err := parseEnvFile(envFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse env file '%s'", envFile)
		}

		for k, v := range envFileVars {
			env[k] = v
		}
	}
	for _, envVar := range envVars {
		env = addEnvVar(env, envVar)
	}
	return env, nil
}

func parseEnvFile(filename string) (map[string]string, error) {
	out := make(map[string]string)
	f, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", filename)
	}
	for _, line := range strings.Split(string(f), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = addEnvVar(out, line)
	}
	return out, nil
}

func addEnvVar(env map[string]string, item string) map[string]string {
	arr := strings.SplitN(item, "=", 2)
	if len(arr) > 1 {
		env[arr[0]] = arr[1]
	} else {
		env[arr[0]] = os.Getenv(arr[0])
	}
	return env
}

func parseProjectToml(appPath, descriptorPath string) (project.Descriptor, string, error) {
	actualPath := descriptorPath
	computePath := descriptorPath == ""

	if computePath {
		actualPath = filepath.Join(appPath, "project.toml")
	}

	if _, err := os.Stat(actualPath); err != nil {
		if computePath {
			return project.Descriptor{}, "", nil
		}
		return project.Descriptor{}, "", errors.Wrap(err, "stat project descriptor")
	}

	descriptor, err := project.ReadProjectDescriptor(actualPath)
	return descriptor, actualPath, err
}

func getFileFilter(descriptor project.Descriptor) (func(string) bool, error) {
	if len(descriptor.Build.Exclude) > 0 {
		excludes, err := ignore.CompileIgnoreLines(descriptor.Build.Exclude...)
		if err != nil {
			return nil, err
		}
		return func(fileName string) bool {
			return !excludes.MatchesPath(fileName)
		}, nil
	}
	if len(descriptor.Build.Include) > 0 {
		includes, err := ignore.CompileIgnoreLines(descriptor.Build.Include...)
		if err != nil {
			return nil, err
		}
		return includes.MatchesPath, nil
	}

	return nil, nil
}
