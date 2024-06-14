package commands

import (
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
)

// Run up to the detect phase of the CNB lifecycle against a source code directory
func ExecuteDetect(logger logging.Logger, cfg config.Config, packClient PackClient) *cobra.Command {
	var flags BuildFlags
	flags.DetectOnly = true

	cmd := &cobra.Command{
		Use:     "detect",
		Args:    cobra.ExactArgs(1),
		Short:   "Execute detect runs the analyze and detect phases of the Cloud Native Buildpacks lifecycle to determine a group of applicable buildpacks and a build plan.",
		Example: "pack execute detect --path apps/test-app --builder cnbs/sample-builder:bionic",
		Long:    "Execute detect uses Cloud Native Buildpacks to run the detect phase of buildpack groups against the source code.\n",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			inputImageName := client.ParseInputImageReference(args[0])
			if err := validateBuildFlags(&flags, cfg, inputImageName, logger); err != nil {
				return err
			}

			descriptor, actualDescriptorPath, err := parseProjectToml(flags.AppPath, flags.DescriptorPath, logger)
			if err != nil {
				return err
			}

			if actualDescriptorPath != "" {
				logger.Debugf("Using project descriptor located at %s", style.Symbol(actualDescriptorPath))
			}

			builder := flags.Builder

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

			var gid = -1
			if cmd.Flags().Changed("gid") {
				gid = flags.GID
			}

			var uid = -1
			if cmd.Flags().Changed("uid") {
				uid = flags.UID
			}

			if err := packClient.Detect(cmd.Context(), client.BuildOptions{
				AppPath:                  flags.AppPath,
				Builder:                  builder,
				Registry:                 flags.Registry,
				Env:                      env,
				Image:                    inputImageName.Name(),
				RunImage:                 flags.RunImage,
				Publish:                  flags.Publish,
				DockerHost:               flags.DockerHost,
				PullPolicy:               pullPolicy,
				ProjectDescriptorBaseDir: filepath.Dir(actualDescriptorPath),
				ProjectDescriptor:        descriptor,

				ContainerConfig: client.ContainerConfig{
					Network: flags.Network,
					Volumes: flags.Volumes,
				},
				LifecycleImage: lifecycleImage,
				PreBuildpacks:  flags.PreBuildpacks,
				PostBuildpacks: flags.PostBuildpacks,
				Buildpacks:     buildpacks,
				Extensions:     extensions,
				Workspace:      flags.Workspace,
				GroupID:        gid,
				UserID:         uid,
				DetectOnly:     true,
			}); err != nil {
				return errors.Wrap(err, "failed to detect")
			}
			return nil
		}),
	}
	buildCommandFlags(cmd, &flags, cfg)
	AddHelpFlag(cmd, "detect")
	return cmd
}
