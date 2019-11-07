package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/internal/config"
	"github.com/buildpack/pack/internal/style"
	"github.com/buildpack/pack/logging"
)

func Rebase(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var opts pack.RebaseOptions
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "rebase <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Rebase app image with latest run image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			opts.RepoName = args[0]
			opts.AdditionalMirrors = getMirrors(cfg)
			if err := client.Rebase(ctx, opts); err != nil {
				return err
			}
			logger.Infof("Successfully rebased image %s", style.Symbol(opts.RepoName))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&opts.Publish, "publish", false, "Publish to registry")
	cmd.Flags().BoolVar(&opts.SkipPull, "no-pull", false, "Skip pulling app and run images before use")
	cmd.Flags().StringVar(&opts.RunImage, "run-image", "", "Run image to use for rebasing")
	AddHelpFlag(cmd, "rebase")
	return cmd
}
