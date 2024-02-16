package commands

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

type DetectFlags struct {
	AppPath        string
	Builder        string
	Registry       string
	DescriptorPath string
	LifecycleImage string
	Volumes        []string
	PreBuildpacks  []string
	PostBuildpacks []string
	Policy         string
	Network        string
	Env            []string
	EnvFiles       []string
	Buildpacks     []string
	Extensions     []string
}

// Run Detect phase of lifecycle against a source code
func Detect(logger logging.Logger, cfg config.Config, packClient PackClient) *cobra.Command {
	var flags DetectFlags

	cmd := &cobra.Command{
		Use:     "detect",
		Args:    cobra.ExactArgs(0),
		Short:   "Run the detect phase of buildpacks against your source code",
		Example: "pack detect --path apps/test-app --builder cnbs/sample-builder:bionic",
		Long: "Pack Detect uses Cloud Native Buildpacks to run the detect phase of buildpack groups against the source code.\n" +
			"You can use `--path` to specify a different source code directory, else it defaults to the current directory. Detect requires a `builder`, which can either " +
			"be provided directly to build using `--builder`, or can be set using the `set-default-builder` command.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := validateDetectFlags(&flags, cfg, logger); err != nil {
				return err
			}

			builder := flags.Builder

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

			if err := packClient.Detect(cmd.Context(), client.BuildOptions{
				AppPath: flags.AppPath,
				Builder: builder,
				Env:     env,

				PullPolicy: pullPolicy,
				ContainerConfig: client.ContainerConfig{
					Network: flags.Network,
					Volumes: flags.Volumes,
				},
				LifecycleImage: lifecycleImage,
				PreBuildpacks:  flags.PreBuildpacks,
				PostBuildpacks: flags.PostBuildpacks,

				Buildpacks: buildpacks,
				Extensions: extensions,
			}); err != nil {
				return errors.Wrap(err, "failed to detect")
			}
			return nil
		}),
	}
	detectCommandFlags(cmd, &flags, cfg)
	AddHelpFlag(cmd, "detect")
	return cmd
}

// TODO: Incomplete
func validateDetectFlags(flags *DetectFlags, cfg config.Config, logger logging.Logger) error {
	// Have to implement
	return nil
}

// TODO: Incomplete
func detectCommandFlags(cmd *cobra.Command, detectFlags *DetectFlags, cfg config.Config) {
	cmd.Flags().StringVarP(&detectFlags.AppPath, "path", "p", "", "Path to app dir or zip-formatted file (defaults to current working directory)")
	cmd.Flags().StringSliceVarP(&detectFlags.Buildpacks, "buildpack", "b", nil, "Buildpack to use. One of:\n  a buildpack by id and version in the form of '<buildpack>@<version>',\n  path to a buildpack directory (not supported on Windows),\n  path/URL to a buildpack .tar or .tgz file, or\n  a packaged buildpack image name in the form of '<hostname>/<repo>[:<tag>]'"+stringSliceHelp("buildpack"))
	cmd.Flags().StringSliceVarP(&detectFlags.Extensions, "extension", "", nil, "Extension to use. One of:\n  an extension by id and version in the form of '<extension>@<version>',\n  path to an extension directory (not supported on Windows),\n  path/URL to an extension .tar or .tgz file, or\n  a packaged extension image name in the form of '<hostname>/<repo>[:<tag>]'"+stringSliceHelp("extension"))
	cmd.Flags().StringVarP(&detectFlags.Builder, "builder", "B", cfg.DefaultBuilder, "Builder image")
	cmd.Flags().StringArrayVarP(&detectFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file."+stringArrayHelp("env")+"\nNOTE: These are NOT available at image runtime.")
	cmd.Flags().StringArrayVar(&detectFlags.EnvFiles, "env-file", []string{}, "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed\nNOTE: These are NOT available at image runtime.\"")
	cmd.Flags().StringVar(&detectFlags.Network, "network", "", "Connect detect and build containers to network")
	cmd.Flags().StringVar(&detectFlags.Policy, "pull-policy", "", `Pull policy to use. Accepted values are always, never, and if-not-present. (default "always")`)
	// cmd.Flags().StringVarP(&detectFlags.DescriptorPath, "descriptor", "d", "", "Path to the project descriptor file")
	cmd.Flags().StringVar(&detectFlags.LifecycleImage, "lifecycle-image", cfg.LifecycleImage, `Custom lifecycle image to use for analysis, restore, and export when builder is untrusted.`)
	cmd.Flags().StringArrayVar(&detectFlags.Volumes, "volume", nil, "Mount host volume into the build container, in the form '<host path>:<target path>[:<options>]'.\n- 'host path': Name of the volume or absolute directory path to mount.\n- 'target path': The path where the file or directory is available in the container.\n- 'options' (default \"ro\"): An optional comma separated list of mount options.\n    - \"ro\", volume contents are read-only.\n    - \"rw\", volume contents are readable and writeable.\n    - \"volume-opt=<key>=<value>\", can be specified more than once, takes a key-value pair consisting of the option name and its value."+stringArrayHelp("volume"))
	cmd.Flags().StringArrayVar(&detectFlags.PreBuildpacks, "pre-buildpack", []string{}, "Buildpacks to prepend to the groups in the builder's order")
	cmd.Flags().StringArrayVar(&detectFlags.PostBuildpacks, "post-buildpack", []string{}, "Buildpacks to append to the groups in the builder's order")
}
