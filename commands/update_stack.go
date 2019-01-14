package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func UpdateStack(logger *logging.Logger) *cobra.Command {
	flags := struct {
		BuildImage string
		RunImages  []string
	}{}
	cmd := &cobra.Command{
		Use:   "update-stack <stack-id> --build-image <build-image-name> --run-image <run-image-name>",
		Args:  cobra.ExactArgs(1),
		Short: "Update stack build and run images",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			if err := cfg.UpdateStack(args[0], config.Stack{
				BuildImage: flags.BuildImage,
				RunImages:  flags.RunImages,
			}); err != nil {
				return err
			}
			logger.Info("Stack %s updated", style.Symbol(args[0]))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.BuildImage, "build-image", "b", "", "Build image to associate with stack")
	cmd.Flags().StringSliceVarP(&flags.RunImages, "run-image", "r", nil, "Run image to associate with stack"+multiValueHelp("run image"))
	AddHelpFlag(cmd, "update-stack")
	return cmd
}
