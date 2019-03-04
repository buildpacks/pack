package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func Build(logger *logging.Logger, dockerClient pack.Docker, imageFactory pack.ImageFactory) *cobra.Command {
	var buildFlags pack.BuildFlags
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "build <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Generate app image from source code",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			buildFlags.RepoName = args[0]

			dockerClient, err := docker.New()
			if err != nil {
				return err
			}
			cacheObj, err := cache.New(buildFlags.RepoName, dockerClient)
			if err != nil {
				return err
			}

			bf, err := pack.DefaultBuildFactory(logger, cacheObj, dockerClient, imageFactory)
			if err != nil {
				return err
			}
			b, err := bf.BuildConfigFromFlags(&buildFlags)
			if err != nil {
				return err
			}
			if err := b.Run(ctx); err != nil {
				return err
			}
			logger.Info("Successfully built image %s", style.Symbol(b.RepoName))
			return nil
		}),
	}
	buildCommandFlags(cmd, &buildFlags)
	cmd.Flags().BoolVar(&buildFlags.Publish, "publish", false, "Publish to registry")
	AddHelpFlag(cmd, "build")
	return cmd
}

func buildCommandFlags(cmd *cobra.Command, buildFlags *pack.BuildFlags) {
	cmd.Flags().StringVarP(&buildFlags.AppDir, "path", "p", "", "Path to app dir (defaults to current working directory)")
	cmd.Flags().StringVar(&buildFlags.Builder, "builder", "", "Builder (defaults to builder configured by 'set-default-builder')")
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringVar(&buildFlags.EnvFile, "env-file", "", "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed")
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "Skip pulling builder and run images before use")
	cmd.Flags().BoolVar(&buildFlags.ClearCache, "clear-cache", false, "Clear image's associated cache before building")
	cmd.Flags().StringSliceVar(&buildFlags.Buildpacks, "buildpack", nil, "Buildpack ID, path to directory, or path/URL to .tgz file"+multiValueHelp("buildpack"))
}
