package commands

import (
	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

//go:generate mockgen -package mocks -destination mocks/builder_inspector.go github.com/buildpack/pack/commands BuilderInspector
type BuilderInspector interface {
	Inspect(image.Image) (pack.Builder, error)
}

func InspectBuilder(logger *logging.Logger, inspector BuilderInspector, imageFactory pack.ImageFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageName := args[0]
			for _, remote := range []bool{true, false} {
				inspectBuilderOutput(logger, imageName, remote, imageFactory, inspector)
				logger.Info("")
			}
			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(logger *logging.Logger, imageName string, remote bool, imageFactory pack.ImageFactory, inspector BuilderInspector) {
	var builderImage image.Image
	var err error
	if remote {
		builderImage, err = imageFactory.NewRemote(imageName)
		logger.Info("Remote\n------")
	} else {
		builderImage, err = imageFactory.NewLocal(imageName, false)
		logger.Info("Local\n-----")
	}
	if err != nil {
		logger.Error(errors.Wrapf(err, "failed to get image %s", style.Symbol(imageName)).Error())
		return
	}
	if found, err := builderImage.Found(); err != nil {
		logger.Error(err.Error())
		return
	} else if !found {
		logger.Info("Not present")
		return
	}

	builder, err := inspector.Inspect(builderImage)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	logger.Info("Run Images:")
	for _, r := range builder.LocalRunImages {
		logger.Info("\t%s (user-configured)", r)
	}
	for _, r := range builder.DefaultRunImages {
		logger.Info("\t%s", r)
	}
}
