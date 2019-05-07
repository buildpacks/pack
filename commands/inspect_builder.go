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

func InspectBuilder(logger logging.LoggerWithWriter, cfg *config.Config, client PackClient) *cobra.Command {
	out := logger.Writer()
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
				_, _ = fmt.Fprintf(out, "Inspecting default builder: %s\n\n", style.Symbol(imageName))
			} else {
				_, _ = fmt.Fprintf(out, "Inspecting builder: %s\n\n", style.Symbol(imageName))
			}

			_, _ = fmt.Fprintln(out, "Remote\n------")
			inspectBuilderOutput(logger, client, imageName, false)

			_, _ = fmt.Fprintln(out, "\nLocal\n-----")
			inspectBuilderOutput(logger, client, imageName, true)

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(logger logging.LoggerWithWriter, client PackClient, imageName string, local bool) {
	out := logger.Writer()
	info, err := client.InspectBuilder(imageName, local)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	if info == nil {
		_, _ = fmt.Fprintln(out, "\nNot present")
		return
	}

	if info.Description != "" {
		logger.Infof("\nDescription: %s", info.Description)
	}

	_, _ = fmt.Fprintf(out, "\nStack: %s\n\n", info.Stack)

	lcycleVer := info.LifecycleVersion
	if lcycleVer == "" {
		lcycleVer = "Unknown"
	}
	_, _ = fmt.Fprintf(out, "Lifecycle Version: %s\n\n", lcycleVer)

	if info.RunImage == "" {
		_, _ = fmt.Fprintf(out, "\nWarning: '%s' does not specify a run image\n", imageName)
		_, _ = fmt.Fprintf(out, "  Users must build with an explicitly specified run image\n")
	} else {
		_, _ = fmt.Fprintln(out, "Run Images:")
		for _, r := range info.LocalRunImageMirrors {
			_, _ = fmt.Fprintf(out, "  %s (user-configured)\n", r)
		}
		_, _ = fmt.Fprintf(out, "  %s\n", info.RunImage)
		for _, r := range info.RunImageMirrors {
			_, _ = fmt.Fprintf(out, "  %s\n", r)
		}
	}

	if len(info.Buildpacks) == 0 {
		_, _ = fmt.Fprintf(out, "\nWarning: '%s' has no buildpacks\n", imageName)
		_, _ = fmt.Fprintln(out, "  Users must supply buildpacks from the host machine")
	} else {
		logBuildpacksInfo(logger, info)
	}

	if len(info.Groups) == 0 {
		_, _ = fmt.Fprintf(out, "\nWarning: '%s' does not specify detection order\n", imageName)
		_, _ = fmt.Fprintln(out, "  Users must build with explicitly specified buildpacks")
	} else {
		logDetectionOrderInfo(logger, info)
	}
}

func logBuildpacksInfo(logger logging.LoggerWithWriter, info *pack.BuilderInfo) {
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

	_, _ = fmt.Fprintln(logger.Writer(), "\nBuildpacks:" + buf.String())
}

func logDetectionOrderInfo(logger logging.LoggerWithWriter, info *pack.BuilderInfo) {
	out := logger.Writer()
	_, _ = fmt.Fprintln(out, "\nDetection Order:")
	for i, group := range info.Groups {
		_, _ = fmt.Fprintf(out, fmt.Sprintf("  Group #%d:\n", i+1))
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
		_, _ = fmt.Fprintln(out, buf.String())
	}
}
