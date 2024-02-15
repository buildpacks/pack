package commands

import (
	"path/filepath"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/cache"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type DetectFlags struct {
	Publish              bool
	ClearCache           bool
	TrustBuilder         bool
	Interactive          bool
	Sparse               bool
	DockerHost           string
	CacheImage           string
	Cache                cache.CacheOpts
	AppPath              string
	Builder              string
	Registry             string
	RunImage             string
	Policy               string
	Network              string
	DescriptorPath       string
	DefaultProcessType   string
	LifecycleImage       string
	Env                  []string
	EnvFiles             []string
	Buildpacks           []string
	Extensions           []string
	Volumes              []string
	AdditionalTags       []string
	Workspace            string
	GID                  int
	UID                  int
	MacAddress           string
	PreviousImage        string
	SBOMDestinationDir   string
	ReportDestinationDir string
	DateTime             string
	PreBuildpacks        []string
	PostBuildpacks       []string
}

// Run Detect phase of lifecycle against a source code
func Detect(logger logging.Logger, cfg config.Config, packClient PackClient) *cobra.Command {
	var flags DetectFlags

	cmd := &cobra.Command{
		Use:     "detect",
		Args:    cobra.ExactArgs(0),
		Short:   "Run the detect phase of buildpacks against the source code",
		Example: "pack detect --path apps/test-app --builder cnbs/sample-builder:bionic",
		Long: "Pack Detect uses Cloud Native Buildpacks to run the detect phase of buildpack groups against the source code.\n\n" +
			"You can use `--path` to specify a different source code directory, else it defaults to the current directory. Detect requires a `builder`, which can either " +
			"be provided directly to build using `--builder`, or can be set using the `set-default-builder` command.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			inputImageName := client.ParseInputImageReference(args[0])
			if err := validateDetectFlags(&flags, cfg, inputImageName, logger); err != nil {
				return err
			}

			// ??
			inputPreviousImage := client.ParseInputImageReference(flags.PreviousImage)

			descriptor, actualDescriptorPath, err := parseProjectToml(flags.AppPath, flags.DescriptorPath, logger)
			if err != nil {
				return err
			}

			// ??
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
				return client.NewSoftError()
			}

			buildpacks := flags.Buildpacks
			extensions := flags.Extensions

			env, err := parseEnv(flags.EnvFiles, flags.Env)
			if err != nil {
				return err
			}

			// ??
			trustBuilder := isTrustedBuilder(cfg, builder) || flags.TrustBuilder

			stringPolicy := flags.Policy
			if stringPolicy == "" {
				stringPolicy = cfg.PullPolicy
			}
			pullPolicy, err := image.ParsePullPolicy(stringPolicy)
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

			if err := packClient.Build(cmd.Context(), client.BuildOptions{
				AppPath:           flags.AppPath,
				Builder:           builder,
				Registry:          flags.Registry,
				AdditionalMirrors: getMirrors(cfg),
				AdditionalTags:    flags.AdditionalTags,
				RunImage:          flags.RunImage,
				Env:               env,
				Image:             inputImageName.Name(),
				Publish:           flags.Publish,
				DockerHost:        flags.DockerHost,
				PullPolicy:        pullPolicy,
				ClearCache:        flags.ClearCache,
				TrustBuilder: func(string) bool {
					return trustBuilder
				},
				Buildpacks: buildpacks,
				Extensions: extensions,
				ContainerConfig: client.ContainerConfig{
					Network: flags.Network,
					Volumes: flags.Volumes,
				},
				DefaultProcessType:       flags.DefaultProcessType,
				ProjectDescriptorBaseDir: filepath.Dir(actualDescriptorPath),
				ProjectDescriptor:        descriptor,
				Cache:                    flags.Cache,
				CacheImage:               flags.CacheImage,
				Workspace:                flags.Workspace,
				LifecycleImage:           lifecycleImage,

				MacAddress:           flags.MacAddress,
				PreviousImage:        inputPreviousImage.Name(),
				Interactive:          flags.Interactive,
				SBOMDestinationDir:   flags.SBOMDestinationDir,
				ReportDestinationDir: flags.ReportDestinationDir,
				PreBuildpacks:        flags.PreBuildpacks,
				PostBuildpacks:       flags.PostBuildpacks,
				LayoutConfig: &client.LayoutConfig{
					Sparse:             flags.Sparse,
					InputImage:         inputImageName,
					PreviousInputImage: inputPreviousImage,
					LayoutRepoDir:      cfg.LayoutRepositoryDir,
				},
			}); err != nil {
				return errors.Wrap(err, "failed to detect")
			}
			return nil
		}),
	}
	detectCommandFlags(cmd, &flags, cfg)
	AddHelpFlag(cmd, "build")
	return cmd
}

// TODO: ,-,
func validateDetectFlags(flags *DetectFlags, cfg config.Config, inputImageRef client.InputImageReference, logger logging.Logger) error {
	if flags.Registry != "" && !cfg.Experimental {
		return client.NewExperimentError("Support for buildpack registries is currently experimental.")
	}

	if flags.Cache.Launch.Format == cache.CacheImage {
		logger.Warn("cache definition: 'launch' cache in format 'image' is not supported.")
	}

	if flags.Cache.Build.Format == cache.CacheImage && flags.CacheImage != "" {
		return errors.New("'cache' flag with 'image' format cannot be used with 'cache-image' flag.")
	}

	if flags.Cache.Build.Format == cache.CacheImage && !flags.Publish {
		return errors.New("image cache format requires the 'publish' flag")
	}

	if flags.CacheImage != "" && !flags.Publish {
		return errors.New("cache-image flag requires the publish flag")
	}

	if flags.GID < 0 {
		return errors.New("gid flag must be in the range of 0-2147483647")
	}

	if flags.UID < 0 {
		return errors.New("uid flag must be in the range of 0-2147483647")
	}

	if flags.MacAddress != "" && !isValidMacAddress(flags.MacAddress) {
		return errors.New("invalid MAC address provided")
	}

	if flags.Interactive && !cfg.Experimental {
		return client.NewExperimentError("Interactive mode is currently experimental.")
	}

	if inputImageRef.Layout() && !cfg.Experimental {
		return client.NewExperimentError("Exporting to OCI layout is currently experimental.")
	}

	return nil
}

// TODO ,-,
func detectCommandFlags(cmd *cobra.Command, detectFlags *DetectFlags, cfg config.Config) {
	cmd.Flags().StringVarP(&detectFlags.AppPath, "path", "p", "", "Path to app dir or zip-formatted file (defaults to current working directory)")
	cmd.Flags().StringSliceVarP(&detectFlags.Buildpacks, "buildpack", "b", nil, "Buildpack to use. One of:\n  a buildpack by id and version in the form of '<buildpack>@<version>',\n  path to a buildpack directory (not supported on Windows),\n  path/URL to a buildpack .tar or .tgz file, or\n  a packaged buildpack image name in the form of '<hostname>/<repo>[:<tag>]'"+stringSliceHelp("buildpack"))
	cmd.Flags().StringSliceVarP(&detectFlags.Extensions, "extension", "", nil, "Extension to use. One of:\n  an extension by id and version in the form of '<extension>@<version>',\n  path to an extension directory (not supported on Windows),\n  path/URL to an extension .tar or .tgz file, or\n  a packaged extension image name in the form of '<hostname>/<repo>[:<tag>]'"+stringSliceHelp("extension"))
	cmd.Flags().StringVarP(&detectFlags.Builder, "builder", "B", cfg.DefaultBuilder, "Builder image")
	cmd.Flags().Var(&detectFlags.Cache, "cache",
		`Cache options used to define cache techniques for build process.
- Cache as bind: 'type=<build/launch>;format=bind;source=<path to directory>'
- Cache as image (requires --publish): 'type=<build/launch>;format=image;name=<registry image name>'
- Cache as volume: 'type=<build/launch>;format=volume;[name=<volume name>]'
    - If no name is provided, a random name will be generated.
`)
	cmd.Flags().StringVar(&detectFlags.CacheImage, "cache-image", "", `Cache build layers in remote registry. Requires --publish`)
	cmd.Flags().BoolVar(&detectFlags.ClearCache, "clear-cache", false, "Clear image's associated cache before building")
	cmd.Flags().StringVar(&detectFlags.DateTime, "creation-time", "", "Desired create time in the output image config. Accepted values are Unix timestamps (e.g., '1641013200'), or 'now'. Platform API version must be at least 0.9 to use this feature.")
	cmd.Flags().StringVarP(&detectFlags.DescriptorPath, "descriptor", "d", "", "Path to the project descriptor file")
	cmd.Flags().StringVarP(&detectFlags.DefaultProcessType, "default-process", "D", "", `Set the default process type. (default "web")`)
	cmd.Flags().StringArrayVarP(&detectFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file."+stringArrayHelp("env")+"\nNOTE: These are NOT available at image runtime.")
	cmd.Flags().StringArrayVar(&detectFlags.EnvFiles, "env-file", []string{}, "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed\nNOTE: These are NOT available at image runtime.\"")
	cmd.Flags().StringVar(&detectFlags.Network, "network", "", "Connect detect and build containers to network")
	cmd.Flags().StringArrayVar(&detectFlags.PreBuildpacks, "pre-buildpack", []string{}, "Buildpacks to prepend to the groups in the builder's order")
	cmd.Flags().StringArrayVar(&detectFlags.PostBuildpacks, "post-buildpack", []string{}, "Buildpacks to append to the groups in the builder's order")
	cmd.Flags().BoolVar(&detectFlags.Publish, "publish", false, "Publish the application image directly to the container registry specified in <image-name>, instead of the daemon. The run image must also reside in the registry.")
	cmd.Flags().StringVar(&detectFlags.DockerHost, "docker-host", "",
		`Address to docker daemon that will be exposed to the build container.
If not set (or set to empty string) the standard socket location will be used.
Special value 'inherit' may be used in which case DOCKER_HOST environment variable will be used.
This option may set DOCKER_HOST environment variable for the build container if needed.
`)
	cmd.Flags().StringVar(&detectFlags.LifecycleImage, "lifecycle-image", cfg.LifecycleImage, `Custom lifecycle image to use for analysis, restore, and export when builder is untrusted.`)
	cmd.Flags().StringVar(&detectFlags.Policy, "pull-policy", "", `Pull policy to use. Accepted values are always, never, and if-not-present. (default "always")`)
	cmd.Flags().StringVarP(&detectFlags.Registry, "buildpack-registry", "r", cfg.DefaultRegistryName, "Buildpack Registry by name")
	cmd.Flags().StringVar(&detectFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringSliceVarP(&detectFlags.AdditionalTags, "tag", "t", nil, "Additional tags to push the output image to.\nTags should be in the format 'image:tag' or 'repository/image:tag'."+stringSliceHelp("tag"))
	cmd.Flags().BoolVar(&detectFlags.TrustBuilder, "trust-builder", false, "Trust the provided builder.\nAll lifecycle phases will be run in a single container.\nFor more on trusted builders, and when to trust or untrust a builder, check out our docs here: https://buildpacks.io/docs/tools/pack/concepts/trusted_builders")
	cmd.Flags().StringArrayVar(&detectFlags.Volumes, "volume", nil, "Mount host volume into the build container, in the form '<host path>:<target path>[:<options>]'.\n- 'host path': Name of the volume or absolute directory path to mount.\n- 'target path': The path where the file or directory is available in the container.\n- 'options' (default \"ro\"): An optional comma separated list of mount options.\n    - \"ro\", volume contents are read-only.\n    - \"rw\", volume contents are readable and writeable.\n    - \"volume-opt=<key>=<value>\", can be specified more than once, takes a key-value pair consisting of the option name and its value."+stringArrayHelp("volume"))
	cmd.Flags().StringVar(&detectFlags.Workspace, "workspace", "", "Location at which to mount the app dir in the build image")
	cmd.Flags().IntVar(&detectFlags.GID, "gid", 0, `Override GID of user's group in the stack's build and run images. The provided value must be a positive number`)
	cmd.Flags().IntVar(&detectFlags.UID, "uid", 0, `Override UID of user in the stack's build and run images. The provided value must be a positive number`)
	cmd.Flags().StringVar(&detectFlags.MacAddress, "mac-address", "", "MAC address to set for the build container network configuration")
	cmd.Flags().StringVar(&detectFlags.PreviousImage, "previous-image", "", "Set previous image to a particular tag reference, digest reference, or (when performing a daemon build) image ID")
	cmd.Flags().StringVar(&detectFlags.SBOMDestinationDir, "sbom-output-dir", "", "Path to export SBoM contents.\nOmitting the flag will yield no SBoM content.")
	cmd.Flags().StringVar(&detectFlags.ReportDestinationDir, "report-output-dir", "", "Path to export build report.toml.\nOmitting the flag yield no report file.")
	cmd.Flags().BoolVar(&detectFlags.Interactive, "interactive", false, "Launch a terminal UI to depict the build process")
	cmd.Flags().BoolVar(&detectFlags.Sparse, "sparse", false, "Use this flag to avoid saving on disk the run-image layers when the application image is exported to OCI layout format")
	if !cfg.Experimental {
		cmd.Flags().MarkHidden("interactive")
		cmd.Flags().MarkHidden("sparse")
	}
}
