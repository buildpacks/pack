package commands

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	pubcfg "github.com/buildpacks/pack/config"

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
	Publish            bool
	NoPull             bool
	ClearCache         bool
	TrustBuilder       bool
	AppPath            string
	Builder            string
	Registry           string
	RunImage           string
	Policy             string
	Network            string
	DescriptorPath     string
	DefaultProcessType string
	Env                []string
	EnvFiles           []string
	Buildpacks         []string
	Volumes            []string
}

// Build an image from source code
func Build(logger logging.Logger, cfg config.Config, packClient PackClient) *cobra.Command {
	var flags BuildFlags

	cmd := &cobra.Command{
		Use:   "build <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Generate app image from source code",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateBuildFlags(&flags, cfg, packClient, logger); err != nil {
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

			trustBuilder := isTrustedBuilder(cfg, flags.Builder) || flags.TrustBuilder
			if trustBuilder {
				logger.Debugf("Builder %s is trusted", style.Symbol(flags.Builder))
			} else {
				logger.Debugf("Builder %s is untrusted", style.Symbol(flags.Builder))
				logger.Debug("As a result, the phases of the lifecycle which require root access will be run in separate trusted ephemeral containers.")
				logger.Debug("For more information, see https://medium.com/buildpacks/faster-more-secure-builds-with-pack-0-11-0-4d0c633ca619")
			}

			if !trustBuilder && len(flags.Volumes) > 0 {
				logger.Warn("Using untrusted builder with volume mounts. If there is sensitive data in the volumes, this may present a security vulnerability.")
			}

			pullPolicy, err := pubcfg.ParsePullPolicy(flags.Policy)
			if err != nil {
				return errors.Wrapf(err, "parsing pull policy %s", flags.Policy)
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
				PullPolicy:        pullPolicy,
				ClearCache:        flags.ClearCache,
				TrustBuilder:      trustBuilder,
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
	AddHelpFlag(cmd, "build")
	return cmd
}

func buildCommandFlags(cmd *cobra.Command, buildFlags *BuildFlags, cfg config.Config) {
	cmd.Flags().StringVarP(&buildFlags.AppPath, "path", "p", "", "Path to app dir or zip-formatted file (defaults to current working directory)")
	cmd.Flags().StringVarP(&buildFlags.Builder, "builder", "B", cfg.DefaultBuilder, "Builder image")
	cmd.Flags().StringVarP(&buildFlags.Registry, "buildpack-registry", "r", cfg.DefaultRegistryName, "Buildpack Registry by name")
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("buildpack-registry")
	}
	cmd.Flags().BoolVar(&buildFlags.Publish, "publish", false, "Publish to registry")
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringArrayVarP(&buildFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file.")
	cmd.Flags().StringArrayVar(&buildFlags.EnvFiles, "env-file", []string{}, "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed")
	cmd.Flags().BoolVar(&buildFlags.ClearCache, "clear-cache", false, "Clear image's associated cache before building")
	cmd.Flags().BoolVar(&buildFlags.TrustBuilder, "trust-builder", false, "Trust the provided builder\nAll lifecycle phases will be run in a single container (if supported by the lifecycle).")
	cmd.Flags().StringSliceVarP(&buildFlags.Buildpacks, "buildpack", "b", nil, "Buildpack reference in the form of '<buildpack>@<version>',\n  path to a buildpack directory (not supported on Windows),\n  path/URL to a buildpack .tar or .tgz file, or\n  the name of a packaged buildpack image"+multiValueHelp("buildpack"))
	cmd.Flags().StringVar(&buildFlags.Network, "network", "", "Connect detect and build containers to network")
	cmd.Flags().StringVarP(&buildFlags.DescriptorPath, "descriptor", "d", "", "Path to the project descriptor file")
	cmd.Flags().StringArrayVar(&buildFlags.Volumes, "volume", nil, "Mount host volume into the build container, in the form '<host path>:<target path>[:<mode>]'."+multiValueHelp("volume"))
	cmd.Flags().StringVarP(&buildFlags.DefaultProcessType, "default-process", "D", "", `Set the default process type. (default "web")`)
	cmd.Flags().StringVar(&buildFlags.Policy, "pull-policy", "", `Pull policy to use. Accepted values are always, never, and if-not-present. (default "always")`)
	// TODO: Remove --no-pull flag after v0.13.0 released. See https://github.com/buildpacks/pack/issues/775
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "Skip pulling builder and run images before use")
	cmd.Flags().MarkHidden("no-pull")
}

func validateBuildFlags(flags *BuildFlags, cfg config.Config, packClient PackClient, logger logging.Logger) error {
	if flags.Builder == "" {
		suggestSettingBuilder(logger, packClient)
		return pack.NewSoftError()
	}

	if flags.Registry != "" && !cfg.Experimental {
		return pack.NewExperimentError("Support for buildpack registries is currently experimental.")
	}

	if flags.NoPull {
		logger.Warn("Flag --no-pull has been deprecated")

		if flags.Policy != "" {
			logger.Warn("Flag --no-pull ignored in favor of --pull-policy")
		} else {
			flags.Policy = pubcfg.PullNever.String()
		}
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
