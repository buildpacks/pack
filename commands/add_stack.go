package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func AddStack(logger *logging.Logger) *cobra.Command {
	flags := struct {
		BuildImage string
		RunImages  []string
	}{}
	cmd := &cobra.Command{
		Use:   "add-stack <stack-id> --build-image <build-image-name> --run-image <run-image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Add stack to list of available stacks",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.AddStack(config.Stack{
				ID:         args[0],
				BuildImage: flags.BuildImage,
				RunImages:  flags.RunImages,
			}); err != nil {
				return err
			}
			logger.Info("Stack %s added", style.Symbol(args[0]))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.BuildImage, "build-image", "b", "", "Build image to associate with stack (required)")
	cmd.MarkFlagRequired("build-image")
	cmd.Flags().StringSliceVarP(&flags.RunImages, "run-image", "r", nil, "Run image to associate with stack (required)"+multiValueHelp("run image"))
	cmd.MarkFlagRequired("run-image")
	AddHelpFlag(cmd, "add-stack")
	return cmd
}
