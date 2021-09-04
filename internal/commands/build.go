package commands

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	pubcfg "github.com/buildpacks/pack/config"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
	"github.com/buildpacks/pack/pkg/project"
	projectTypes "github.com/buildpacks/pack/pkg/project/types"
)

type BuildFlags struct {
	Publish            bool
	ClearCache         bool
	TrustBuilder       bool
	Interactive        bool
	DockerHost         string
	CacheImage         string
	AppPath            string
	Builder            string
	Registry           string
	RunImage           string
	Policy             string
	Network            string
	DescriptorPath     string
	DefaultProcessType string
	LifecycleImage     string
	Env                []string
	EnvFiles           []string
	Buildpacks         []string
	Volumes            []string
	AdditionalTags     []string
	Workspace          string
	GID                int
	PreviousImage      string
}

// Build an image from source code
func Build(logger logging.Logger, cfg config.Config, packClient PackClient) *cobra.Command {
	var flags BuildFlags

	cmd := &cobra.Command{
		Use:     "build <image-name>",
		Args:    cobra.ExactArgs(1),
		Short:   "Generate app image from source code",
		Example: "pack build test_img --path apps/test-app --builder cnbs/sample-builder:bionic",
		Long: "Pack Build uses Cloud Native Buildpacks to create a runnable  app image from source code.\n\nPack Build " +
			"requires an image name, which will be generated from the source code. Build defaults to the current directory, " +
			"but you can use `--path` to specify another source code directory. Build requires a `builder`, which can either " +
			"be provided directly to build using `--builder`, or can be set using the `set-default-builder` command. For more " +
			"on how to use `pack build`, see: https://buildpacks.io/docs/app-developer-guide/build-an-app/.",
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

			builder := flags.Builder
			// We only override the builder to the one in the project descriptor
			// if it was not explicitly set by the user
			if !cmd.Flags().Changed("builder") && descriptor.Build.Builder != "" {
				builder = descriptor.Build.Builder
			}

			if builder == "" {
				suggestSettingBuilder(logger, packClient)
				return pack.NewSoftError()
			}

			buildpacks := flags.Buildpacks

			env, err := parseEnv(flags.EnvFiles, flags.Env)
			if err != nil {
				return err
			}

			trustBuilder := isTrustedBuilder(cfg, builder) || flags.TrustBuilder
			if trustBuilder {
				logger.Debugf("Builder %s is trusted", style.Symbol(builder))
			} else {
				logger.Debugf("Builder %s is untrusted", style.Symbol(builder))
				logger.Debug("As a result, the phases of the lifecycle which require root access will be run in separate trusted ephemeral containers.")
				logger.Debug("For more information, see https://medium.com/buildpacks/faster-more-secure-builds-with-pack-0-11-0-4d0c633ca619")
			}

			if !trustBuilder && len(flags.Volumes) > 0 {
				logger.Warn("Using untrusted builder with volume mounts. If there is sensitive data in the volumes, this may present a security vulnerability.")
			}

			stringPolicy := flags.Policy
			if stringPolicy == "" {
				stringPolicy = cfg.PullPolicy
			}
			pullPolicy, err := pubcfg.ParsePullPolicy(stringPolicy)
			if err != nil {
				return errors.Wrapf(err, "parsing pull policy %s", flags.Policy)
			}
			var lifecycleImage string
			if flags.LifecycleImage != "" {
				ref, err := name.ParseReference(flags.LifecycleImage)
				if err != nil {
					return errors.Wrapf(err, "parsing lifecycle image %s", flags.LifecycleImage)
				}
				lifecycleImage = ref.Name()
			}
			var gid = -1
			if cmd.Flags().Changed("gid") {
				gid = flags.GID
			}
			if err := packClient.Build(cmd.Context(), pack.BuildOptions{
				AppPath:           flags.AppPath,
				Builder:           builder,
				Registry:          flags.Registry,
				AdditionalMirrors: getMirrors(cfg),
				AdditionalTags:    flags.AdditionalTags,
				RunImage:          flags.RunImage,
				Env:               env,
				Image:             imageName,
				Publish:           flags.Publish,
				DockerHost:        flags.DockerHost,
				PullPolicy:        pullPolicy,
				ClearCache:        flags.ClearCache,
				TrustBuilder: func() bool {
					return trustBuilder
				},
				Buildpacks: buildpacks,
				ContainerConfig: pack.ContainerConfig{
					Network: flags.Network,
					Volumes: flags.Volumes,
				},
				DefaultProcessType:       flags.DefaultProcessType,
				ProjectDescriptorBaseDir: filepath.Dir(actualDescriptorPath),
				ProjectDescriptor:        descriptor,
				CacheImage:               flags.CacheImage,
				Workspace:                flags.Workspace,
				LifecycleImage:           lifecycleImage,
				GroupID:                  gid,
				PreviousImage:            flags.PreviousImage,
				Interactive:              flags.Interactive,
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
	cmd.Flags().StringSliceVarP(&buildFlags.Buildpacks, "buildpack", "b", nil, "Buildpack to use. One of:\n  a buildpack by id and version in the form of '<buildpack>@<version>',\n  path to a buildpack directory (not supported on Windows),\n  path/URL to a buildpack .tar or .tgz file, or\n  a packaged buildpack image name in the form of '<hostname>/<repo>[:<tag>]'"+multiValueHelp("buildpack"))
	cmd.Flags().StringVarP(&buildFlags.Builder, "builder", "B", cfg.DefaultBuilder, "Builder image")
	cmd.Flags().StringVar(&buildFlags.CacheImage, "cache-image", "", `Cache build layers in remote registry. Requires --publish`)
	cmd.Flags().BoolVar(&buildFlags.ClearCache, "clear-cache", false, "Clear image's associated cache before building")
	cmd.Flags().StringVarP(&buildFlags.DescriptorPath, "descriptor", "d", "", "Path to the project descriptor file")
	cmd.Flags().StringVarP(&buildFlags.DefaultProcessType, "default-process", "D", "", `Set the default process type. (default "web")`)
	cmd.Flags().StringArrayVarP(&buildFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file."+multiValueHelp("env")+"\nNOTE: These are NOT available at image runtime.")
	cmd.Flags().StringArrayVar(&buildFlags.EnvFiles, "env-file", []string{}, "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed\nNOTE: These are NOT available at image runtime.\"")
	cmd.Flags().StringVar(&buildFlags.Network, "network", "", "Connect detect and build containers to network")
	cmd.Flags().BoolVar(&buildFlags.Publish, "publish", false, "Publish to registry")
	cmd.Flags().StringVar(&buildFlags.DockerHost, "docker-host", "",
		`Address to docker daemon that will be exposed to the build container.
If not set (or set to empty string) the standard socket location will be used.
Special value 'inherit' may be used in which case DOCKER_HOST environment variable will be used.
This option may set DOCKER_HOST environment variable for the build container if needed.
`)
	cmd.Flags().StringVar(&buildFlags.LifecycleImage, "lifecycle-image", cfg.LifecycleImage, `Custom lifecycle image to use for analysis, restore, and export when builder is untrusted.`)
	cmd.Flags().StringVar(&buildFlags.Policy, "pull-policy", "", `Pull policy to use. Accepted values are always, never, and if-not-present. (default "always")`)
	cmd.Flags().StringVarP(&buildFlags.Registry, "buildpack-registry", "r", cfg.DefaultRegistryName, "Buildpack Registry by name")
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringSliceVarP(&buildFlags.AdditionalTags, "tag", "t", nil, "Additional tags to push the output image to.\nTags should be in the format 'image:tag' or 'repository/image:tag'."+multiValueHelp("tag"))
	cmd.Flags().BoolVar(&buildFlags.TrustBuilder, "trust-builder", false, "Trust the provided builder\nAll lifecycle phases will be run in a single container (if supported by the lifecycle).")
	cmd.Flags().StringArrayVar(&buildFlags.Volumes, "volume", nil, "Mount host volume into the build container, in the form '<host path>:<target path>[:<options>]'.\n- 'host path': Name of the volume or absolute directory path to mount.\n- 'target path': The path where the file or directory is available in the container.\n- 'options' (default \"ro\"): An optional comma separated list of mount options.\n    - \"ro\", volume contents are read-only.\n    - \"rw\", volume contents are readable and writeable.\n    - \"volume-opt=<key>=<value>\", can be specified more than once, takes a key-value pair consisting of the option name and its value."+multiValueHelp("volume"))
	cmd.Flags().StringVar(&buildFlags.Workspace, "workspace", "", "Location at which to mount the app dir in the build image")
	cmd.Flags().IntVar(&buildFlags.GID, "gid", 0, `Override GID of user's group in the stack's build and run images. The provided value must be a positive number`)
	cmd.Flags().StringVar(&buildFlags.PreviousImage, "previous-image", "", "Set previous image to a particular tag reference, digest reference, or (when performing a daemon build) image ID")
	cmd.Flags().BoolVar(&buildFlags.Interactive, "interactive", false, "Launch a terminal UI to depict the build process")
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("interactive")
	}
}

func validateBuildFlags(flags *BuildFlags, cfg config.Config, packClient PackClient, logger logging.Logger) error {
	if flags.Registry != "" && !cfg.Experimental {
		return pack.NewExperimentError("Support for buildpack registries is currently experimental.")
	}

	if flags.CacheImage != "" && !flags.Publish {
		return errors.New("cache-image flag requires the publish flag")
	}

	if flags.GID < 0 {
		return errors.New("gid flag must be in the range of 0-2147483647")
	}

	if flags.Interactive && !cfg.Experimental {
		return pack.NewExperimentError("Interactive mode is currently experimental.")
	}

	return nil
}

func parseEnv(envFiles []string, envVars []string) (map[string]string, error) {
	env := map[string]string{}

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
	f, err := ioutil.ReadFile(filepath.Clean(filename))
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

func parseProjectToml(appPath, descriptorPath string) (projectTypes.Descriptor, string, error) {
	actualPath := descriptorPath
	computePath := descriptorPath == ""

	if computePath {
		actualPath = filepath.Join(appPath, "project.toml")
	}

	if _, err := os.Stat(actualPath); err != nil {
		if computePath {
			return projectTypes.Descriptor{}, "", nil
		}
		return projectTypes.Descriptor{}, "", errors.Wrap(err, "stat project descriptor")
	}

	descriptor, err := project.ReadProjectDescriptor(actualPath)
	return descriptor, actualPath, err
}
