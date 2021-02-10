package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder/writer"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

type BuilderInspector interface {
	InspectBuilder(name string, daemon bool, modifiers ...pack.BuilderInspectionModifier) (*pack.BuilderInfo, error)
}

type BuilderInspectFlags struct {
	Depth        int
	OutputFormat string
}

func BuilderInspect(logger logging.Logger,
	cfg config.Config,
	inspector BuilderInspector,
	writerFactory writer.BuilderWriterFactory,
) *cobra.Command {
	var flags BuilderInspectFlags
	cmd := &cobra.Command{
		Use:     "inspect <builder-image-name>",
		Args:    cobra.MaximumNArgs(1),
		Aliases: []string{"inspect-builder"},
		Short:   "Show information about a builder",
		Example: "pack builder inspect cnbs/sample-builder:bionic",
		Long:    "Show information about the builder provided. If no argument is provided, it will inspect the default builder, if one has been set.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageName := cfg.DefaultBuilder
			if len(args) >= 1 {
				imageName = args[0]
			}

			if imageName == "" {
				suggestSettingBuilder(logger, inspector)
				return pack.NewSoftError()
			}

			return inspectBuilder(logger, imageName, flags, cfg, inspector, writerFactory)
		}),
	}

	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", builder.OrderDetectionMaxDepth, "Max depth to display for Detection Order.\nOmission of this flag or values < 0 will display the entire tree.")
	cmd.Flags().StringVarP(&flags.OutputFormat, "output", "o", "human-readable", "Output format to display builder detail (json, yaml, toml, human-readable).\nOmission of this flag will display as human-readable.")
	AddHelpFlag(cmd, "inspect")
	return cmd
}

func inspectBuilder(
	logger logging.Logger,
	imageName string,
	flags BuilderInspectFlags,
	cfg config.Config,
	inspector BuilderInspector,
	writerFactory writer.BuilderWriterFactory,
) error {
	builderInfo := writer.SharedBuilderInfo{
		Name:      imageName,
		IsDefault: imageName == cfg.DefaultBuilder,
		Trusted:   isTrustedBuilder(cfg, imageName),
	}

	localInfo, localErr := inspector.InspectBuilder(imageName, true, pack.WithDetectionOrderDepth(flags.Depth))
	remoteInfo, remoteErr := inspector.InspectBuilder(imageName, false, pack.WithDetectionOrderDepth(flags.Depth))

	writer, err := writerFactory.Writer(flags.OutputFormat)
	if err != nil {
		return err
	}
	return writer.Print(logger, cfg.RunImages, localInfo, remoteInfo, localErr, remoteErr, builderInfo)
}
