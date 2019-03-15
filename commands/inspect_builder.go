package commands

import (
	"bytes"
	"fmt"
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"text/tabwriter"
)

//go:generate mockgen -package mocks -destination mocks/builder_inspector.go github.com/buildpack/pack/commands BuilderInspector
type BuilderInspector interface {
	Inspect(image.Image) (pack.Builder, error)
}

func InspectBuilder(logger *logging.Logger, cfg *config.Config, inspector BuilderInspector, fetcher pack.Fetcher) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageName := cfg.DefaultBuilder
			if len(args) >= 1 {
				imageName = args[0]
			}

			if imageName == cfg.DefaultBuilder {
				logger.Info("Inspecting default builder: %s\n", imageName)
			} else {
				logger.Info("Inspecting builder: %s\n", imageName)
			}

			inspectBuilderOutput(logger, imageName, true, inspector, fetcher)
			logger.Info("")
			inspectBuilderOutput(logger, imageName, false, inspector, fetcher)

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(logger *logging.Logger, imageName string, remote bool, inspector BuilderInspector, fetcher pack.Fetcher) {
	var (
		err          error
		builderImage image.Image
	)

	if remote {
		builderImage, err = fetcher.FetchRemoteImage(imageName)
		logger.Info("Remote\n------\n")
	} else {
		builderImage, err = fetcher.FetchLocalImage(imageName)
		logger.Info("Local\n-----\n")
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

	stack, err := builderImage.Label("io.buildpacks.stack.id")
	if err != nil {
		logger.Error(err.Error())
		return
	}

	if stack == "" {
		logger.Error("Error: '%s' is an invalid builder because it is missing a 'io.buildpacks.stack.id' label", imageName)
		return
	}

	logger.Info("Stack: %s\n", stack)

	builder, err := inspector.Inspect(builderImage)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	logger.Info("Run Images:")
	for _, r := range builder.LocalRunImageMirrors {
		logger.Info("  %s (user-configured)", r)
	}
	logger.Info("  %s", builder.RunImage)
	for _, r := range builder.RunImageMirrors {
		logger.Info("  %s", r)
	}

	if len(builder.Buildpacks) == 0 {
		logger.Info("\nWarning: '%s' has no buildpacks", imageName)
		logger.Info("  Users must supply buildpacks from the host machine")
	} else {
		logBuildpacksInfo(logger, builder)
	}

	if len(builder.Groups) == 0 {
		logger.Info("\nWarning: '%s' does not specify detection order", imageName)
		logger.Info("  Users must build with explicitly specified buildpacks")
	} else {
		logDetectionOrderInfo(logger, builder)
	}
}

func logBuildpacksInfo(logger *logging.Logger, builder pack.Builder) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 8, ' ', 0)
	if _, err := fmt.Fprint(tabWriter, "\n  ID\tVERSION\tLATEST\t"); err != nil {
		logger.Error(err.Error())
	}

	for _, bp := range builder.Buildpacks {
		if _, err := fmt.Fprint(tabWriter, fmt.Sprintf("\n  %s\t%s\t%t\t", bp.ID, bp.Version, bp.Latest)); err != nil {
			logger.Error(err.Error())
		}
	}

	if err := tabWriter.Flush(); err != nil {
		logger.Error(err.Error())
	}

	logger.Info("\nBuildpacks:" + buf.String())
}

func logDetectionOrderInfo(logger *logging.Logger, builder pack.Builder) {
	logger.Info("\nDetection Order:")
	for i, group := range builder.Groups {
		logger.Info(fmt.Sprintf("  Group #%d:", i+1))
		for _, bp := range group.Buildpacks {
			logger.Info(fmt.Sprintf("    %s@%s", bp.ID, bp.Version))
		}
	}
}
