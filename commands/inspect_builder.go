package commands

import (
	"bytes"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func InspectBuilder(logger *logging.Logger, cfg *config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if cfg.DefaultBuilder == "" && len(args) == 0 {
				suggestSettingBuilder(logger, client)
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

			logger.Info("Remote\n------")
			inspectBuilderOutput(logger, client, imageName, false)

			logger.Info("\nLocal\n-----")
			inspectBuilderOutput(logger, client, imageName, true)

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(logger *logging.Logger, client PackClient, imageName string, local bool) {
	info, err := client.InspectBuilder(imageName, local)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	if info == nil {
		logger.Info("\nNot present")
		return
	}

	if info.Description != "" {
		logger.Info("\nDescription: %s", info.Description)
	}

	logger.Info("\nStack: %s\n", info.Stack)

	lcycleVer := info.LifecycleVersion
	if lcycleVer == "" {
		lcycleVer = "Unknown"
	}
	logger.Info("Lifecycle Version: %s\n", lcycleVer)

	if info.RunImage == "" {
		logger.Info("\nWarning: '%s' does not specify a run image", imageName)
		logger.Info("  Users must build with an explicitly specified run image")
	} else {
		logger.Info("Run Images:")
		for _, r := range info.LocalRunImageMirrors {
			logger.Info("  %s (user-configured)", r)
		}
		logger.Info("  %s", info.RunImage)
		for _, r := range info.RunImageMirrors {
			logger.Info("  %s", r)
		}
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
	if _, err := fmt.Fprint(tabWriter, "\n  ID\tVERSION\tLATEST"); err != nil {
		logger.Error(err.Error())
	}

	for _, bp := range info.Buildpacks {
		if _, err := fmt.Fprint(tabWriter, fmt.Sprintf("\n  %s\t%s\t%t", bp.ID, bp.Version, bp.Latest)); err != nil {
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
		buf := &bytes.Buffer{}
		tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 4, ' ', 0)
		for i, bp := range group.Buildpacks {
			var optional string
			if bp.Optional {
				optional = "(optional)"
			}
			if _, err := fmt.Fprintf(tabWriter, "    %s@%s\t%s", bp.ID, bp.Version, optional); err != nil {
				logger.Error(err.Error())
			}
			if i < len(group.Buildpacks)-1 {
				if _, err := fmt.Fprint(tabWriter, "\n"); err != nil {
					logger.Error(err.Error())
				}
			}
		}
		if err := tabWriter.Flush(); err != nil {
			logger.Error(err.Error())
		}
		logger.Info(buf.String())
	}
}
