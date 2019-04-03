package commands

import (
	"bytes"
	"fmt"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"text/tabwriter"
)

//go:generate mockgen -package mocks -destination mocks/inspect_builder.go github.com/buildpack/pack/commands BuilderInspector
type BuilderInspector interface {
	InspectBuilder(string, bool) (*pack.BuilderInfo, error)
}

func InspectBuilder(logger *logging.Logger, cfg *config.Config, inspector BuilderInspector) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if cfg.DefaultBuilder == "" && len(args) == 0 {
				suggestSettingBuilder(logger)
				return MakeSoftError()
			}

			imageName := cfg.DefaultBuilder
			if len(args) >= 1 {
				imageName = args[0]
			}

			if imageName == cfg.DefaultBuilder {
				logger.Info("Inspecting default builder: %s\n", style.Symbol(imageName))
			} else {
				logger.Info("Inspecting builder: %s\n", style.Symbol(imageName))
			}

			logger.Info("Remote\n------\n")
			inspectBuilderOutput(logger, inspector, imageName, false)

			logger.Info("\nLocal\n-----\n")
			inspectBuilderOutput(logger, inspector, imageName, true)

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(logger *logging.Logger, inspector BuilderInspector, imageName string, local bool) {
	info, err := inspector.InspectBuilder(imageName, local)
	if err != nil {
		logger.Error(errors.Wrapf(err, "failed to inspect image %s", style.Symbol(imageName)).Error())
		return
	}

	if info == nil {
		logger.Info("Not present")
		return
	}

	logger.Info("Stack: %s\n", info.Stack)

	logger.Info("Run Images:")
	for _, r := range info.LocalRunImageMirrors {
		logger.Info("  %s (user-configured)", r)
	}
	logger.Info("  %s", info.RunImage)
	for _, r := range info.RunImageMirrors {
		logger.Info("  %s", r)
	}

	if len(info.Buildpacks) == 0 {
		logger.Info("\nWarning: '%s' has no buildpacks", imageName)
		logger.Info("  Users must supply buildpacks from the host machine")
	} else {
		logBuildpacksInfo(logger, info)
	}

	if len(info.Groups) == 0 {
		logger.Info("\nWarning: '%s' does not specify detection order", imageName)
		logger.Info("  Users must build with explicitly specified buildpacks")
	} else {
		logDetectionOrderInfo(logger, info)
	}
}

func logBuildpacksInfo(logger *logging.Logger, info *pack.BuilderInfo) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 8, ' ', 0)
	if _, err := fmt.Fprint(tabWriter, "\n  ID\tVERSION\tLATEST\t"); err != nil {
		logger.Error(err.Error())
	}

	for _, bp := range info.Buildpacks {
		if _, err := fmt.Fprint(tabWriter, fmt.Sprintf("\n  %s\t%s\t%t\t", bp.ID, bp.Version, bp.Latest)); err != nil {
			logger.Error(err.Error())
		}
	}

	if err := tabWriter.Flush(); err != nil {
		logger.Error(err.Error())
	}

	logger.Info("\nBuildpacks:" + buf.String())
}

func logDetectionOrderInfo(logger *logging.Logger, info *pack.BuilderInfo) {
	logger.Info("\nDetection Order:")
	for i, group := range info.Groups {
		logger.Info(fmt.Sprintf("  Group #%d:", i+1))
		for _, bp := range group {
			logger.Info(fmt.Sprintf("    %s@%s", bp.ID, bp.Version))
		}
	}
}
