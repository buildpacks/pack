package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func Rebase(logger *logging.Logger, fetcher pack.Fetcher) *cobra.Command {
	var flags pack.RebaseFlags
	ctx := createCancellableContext()

	cmd := &cobra.Command{
		Use:   "rebase <image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Rebase app image with latest run image",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			flags.RepoName = args[0]
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			factory := pack.RebaseFactory{
				Logger:  logger,
				Config:  cfg,
				Fetcher: fetcher,
			}
			rebaseConfig, err := factory.RebaseConfigFromFlags(ctx, flags, logger.VerboseWriter().WithPrefix("docker"))
			if err != nil {
				return err
			}
			if err := factory.Rebase(rebaseConfig); err != nil {
				return err
			}
			logger.Info("Successfully rebased image %s", style.Symbol(rebaseConfig.Image.Name()))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&flags.Publish, "publish", false, "Publish to registry")
	cmd.Flags().BoolVar(&flags.NoPull, "no-pull", false, "Skip pulling app and run images before use")
	cmd.Flags().StringVar(&flags.RunImage, "run-image", "", "Run image to use for rebasing")
	AddHelpFlag(cmd, "rebase")
	return cmd
}
