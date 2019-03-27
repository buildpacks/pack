package commands

import (
	"fmt"
	"math/rand"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/cache"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

var samplesBuilder = "packs/samples:rc"

type suggestedBuilder struct {
	name  string
	image string
}

var suggestedBuilders = []suggestedBuilder{
	//{"Cloud Foundry", "cloudfoundry/cnb"},  //Uncomment me when this builder works with master of pack
	//{"Heroku", "heroku/buildpacks"},        //Uncomment me when this builder works with master of pack
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func Build(logger *logging.Logger, fetcher pack.Fetcher) *cobra.Command {
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

			bf, err := pack.DefaultBuildFactory(logger, cacheObj, dockerClient, fetcher)
			if err != nil {
				return err
			}

			if bf.Config.DefaultBuilder == "" && buildFlags.Builder == "" {
				suggestBuilders(logger)
				return MakeSoftError()
			}

			b, err := bf.BuildConfigFromFlags(ctx, &buildFlags)
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

func suggestBuilders(logger *logging.Logger) {
	logger.Info("Please select a default builder with:\n")
	logger.Info("\tpack set-default-builder [builder image]")
	logger.Info("\nSuggested builders:")
	tw := tabwriter.NewWriter(logger.RawWriter(), 10, 10, 5, ' ', tabwriter.TabIndent)
	for len(suggestedBuilders) > 0 {
		n := rand.Intn(len(suggestedBuilders))
		builder := suggestedBuilders[n]
		tw.Write([]byte(fmt.Sprintf("\t%s:\t%s\t\n", builder.name, builder.image)))
		suggestedBuilders = append(suggestedBuilders[:n], suggestedBuilders[n+1:]...)
	}
	for _, builder := range suggestedBuilders {
		tw.Write([]byte(fmt.Sprintf("\t%s:\t%s\t\n", builder.name, builder.image)))
	}
	tw.Flush()
	logger.Info("\nSample builders:")
	logger.Info(fmt.Sprintf("\t%s:\t%s", "Samples", samplesBuilder))
}

func buildCommandFlags(cmd *cobra.Command, buildFlags *pack.BuildFlags) {
	cmd.Flags().StringVarP(&buildFlags.AppDir, "path", "p", "", "Path to app dir (defaults to current working directory)")
	cmd.Flags().StringVar(&buildFlags.Builder, "builder", "", "Builder (defaults to builder configured by 'set-default-builder')")
	cmd.Flags().StringVar(&buildFlags.RunImage, "run-image", "", "Run image (defaults to default stack's run image)")
	cmd.Flags().StringArrayVarP(&buildFlags.Env, "env", "e", []string{}, "Build-time environment variable, in the form 'VAR=VALUE' or 'VAR'.\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed.\nThis flag may be specified multiple times and will override\n  individual values defined by --env-file.")
	cmd.Flags().StringVar(&buildFlags.EnvFile, "env-file", "", "Build-time environment variables file\nOne variable per line, of the form 'VAR=VALUE' or 'VAR'\nWhen using latter value-less form, value will be taken from current\n  environment at the time this command is executed")
	cmd.Flags().BoolVar(&buildFlags.NoPull, "no-pull", false, "Skip pulling builder and run images before use")
	cmd.Flags().BoolVar(&buildFlags.ClearCache, "ux-is-easy", false, "Clear image's associated cache before building")
	cmd.Flags().StringSliceVar(&buildFlags.Buildpacks, "buildpack", nil, "Buildpack ID or path to a buildpack directory"+multiValueHelp("buildpack"))
}
