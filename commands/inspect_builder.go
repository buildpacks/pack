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

func InspectBuilder(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
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
				logger.Infof("Inspecting default builder: %s", style.Symbol(imageName))
				logger.Info("")
			} else {
				logger.Infof("Inspecting builder: %s", style.Symbol(imageName))
				logger.Info("")
			}

			logger.Info("Remote")
			logger.Info("------")
			inspectBuilderOutput(logger, client, imageName, false, cfg)

			logger.Info("")
			logger.Info("Local")
			logger.Info("-----")
			inspectBuilderOutput(logger, client, imageName, true, cfg)

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

// TODO: If necessary, how do we present buildpack order (nested order)?
func inspectBuilderOutput(logger logging.Logger, client PackClient, imageName string, local bool, cfg config.Config) {
	info, err := client.InspectBuilder(imageName, local)
	if err != nil {
		logger.Info("")
		logger.Error(err.Error())
		return
	}

	if info == nil {
		logger.Info("")
		logger.Info("Not present")
		return
	}

	if info.Description != "" {
		logger.Infof("\nDescription: %s", info.Description)
	}

	logger.Info("")
	logger.Infof("Stack: %s", info.Stack)
	logger.Info("")

	lcycleVer := info.LifecycleVersion
	if lcycleVer == "" {
		lcycleVer = "Unknown"
	}
	logger.Infof("Lifecycle Version: %s", lcycleVer)
	logger.Info("")

	if info.RunImage == "" {
		logger.Infof("\nWarning: '%s' does not specify a run image", imageName)
		logger.Info("  Users must build with an explicitly specified run image")
	} else {
		logger.Info("Run Images:")

		for _, r := range getLocalMirrors(info.RunImage, cfg) {
			logger.Infof("  %s (user-configured)", r)
		}
		logger.Infof("  %s", info.RunImage)
		for _, r := range info.RunImageMirrors {
			logger.Infof("  %s", r)
		}
	}

	if len(info.Buildpacks) == 0 {
		logger.Infof("\nWarning: '%s' has no buildpacks", imageName)
		logger.Info("  Users must supply buildpacks from the host machine")
	} else {
		logBuildpacksInfo(logger, info)
	}

	if len(info.Groups) == 0 {
		logger.Infof("\nWarning: '%s' does not specify detection order", imageName)
		logger.Info("  Users must build with explicitly specified buildpacks")
	} else {
		logDetectionOrderInfo(logger, info)
	}
}

func logBuildpacksInfo(logger logging.Logger, info *pack.BuilderInfo) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 8, ' ', 0)
	if _, err := fmt.Fprint(tabWriter, "\n  ID\tVERSION"); err != nil {
		logger.Error(err.Error())
	}

	for _, bp := range info.Buildpacks {
		if _, err := fmt.Fprint(tabWriter, fmt.Sprintf("\n  %s\t%s", bp.ID, bp.Version)); err != nil {
			logger.Error(err.Error())
		}
	}

	if err := tabWriter.Flush(); err != nil {
		logger.Error(err.Error())
	}

	logger.Info("\nBuildpacks:" + buf.String())
}

func logDetectionOrderInfo(logger logging.Logger, info *pack.BuilderInfo) {
	logger.Info("\nDetection Order:")
	for i, group := range info.Groups {
		logger.Infof("  Group #%d:", i+1)
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

func getLocalMirrors(runImage string, cfg config.Config) []string {
	for _, ri := range cfg.RunImages {
		if ri.Image == runImage {
			return ri.Mirrors
		}
	}
	return nil
}
