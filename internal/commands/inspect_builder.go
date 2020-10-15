package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

type BuilderWriter interface {
	Print(
		logger logging.Logger,
		localRunImages []config.RunImage,
		local, remote *pack.BuilderInfo,
		localErr, remoteErr error,
		builderInfo SharedBuilderInfo,
	) error
}

type BuilderWriterFactory interface {
	Writer(kind string) (BuilderWriter, error)
}

type BuilderInspector interface {
	InspectBuilder(name string, daemon bool, modifiers ...pack.BuilderInspectionModifier) (*pack.BuilderInfo, error)
}

type InspectBuilderFlags struct {
	Depth        int
	OutputFormat string
}

type SharedBuilderInfo struct {
	Name      string `json:"builder_name" yaml:"builder_name" toml:"builder_name"`
	Trusted   bool   `json:"trusted" yaml:"trusted" toml:"trusted"`
	IsDefault bool   `json:"default" yaml:"default" toml:"default"`
}

func InspectBuilder(
	logger logging.Logger,
	cfg config.Config,
	inspector BuilderInspector,
	writerFactory BuilderWriterFactory,
) *cobra.Command {
	var flags InspectBuilderFlags
	cmd := &cobra.Command{
		Use:     "inspect-builder <builder-image-name>",
		Args:    cobra.MaximumNArgs(2),
		Short:   "Show information about a builder",
		Example: "pack inspect-builder cnbs/sample-builder:bionic",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			imageName := cfg.DefaultBuilder
			if len(args) >= 1 {
				imageName = args[0]
			}

			if imageName == "" {
				suggestSettingBuilder(logger, inspector)
				return pack.NewSoftError()
			}

			builderInfo := SharedBuilderInfo{
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
		}),
	}
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", builder.OrderDetectionMaxDepth, "Max depth to display for Detection Order.\nOmission of this flag or values < 0 will display the entire tree.")
	cmd.Flags().StringVarP(&flags.OutputFormat, "output", "o", "human-readable", "Output format to display builder detail (json, yaml, toml, human-readable).\nOmission of this flag will display as human-readable.")
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}
