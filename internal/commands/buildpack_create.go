package commands

import (
	"context"

	scafall "github.com/buildpacks/scafall/pkg"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

const (
	CNBTemplates = "https://github.com/buildpacks/templates"
)

type BuildpackCreateFlags struct {
	Arguments map[string]string
	Template  string
	SubPath   string
}

type BuildpackCreateCreator interface {
	CreateBuildpack(ctx context.Context, options client.CreateBuildpackOptions) error
}

func BuildpackCreate(logger logging.Logger, creator BuildpackCreateCreator) *cobra.Command {
	flags := BuildpackCreateFlags{}
	cmd := &cobra.Command{
		Use:     "create",
		Short:   "(Experimental) Creates basic scaffolding of a buildpack.",
		Args:    cobra.MatchAll(cobra.ExactArgs(0)),
		Example: "pack buildpack create",
		Long:    "buildpack create generates the basic scaffolding of a buildpack repository.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := creator.CreateBuildpack(cmd.Context(), client.CreateBuildpackOptions{
				Template:  flags.Template,
				SubPath:   flags.SubPath,
				Arguments: flags.Arguments,
			}); err != nil {
				return err
			}

			logger.Infof("Successfully scaffolded %s", style.Symbol("template"))
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.Template, "template", "t", CNBTemplates, "URL of the buildpack template git repository")
	cmd.Flags().StringVar(&flags.SubPath, "sub-path", "", "directory within template git repository used to generate the buildpack")
	cmd.Flags().StringToStringVarP(&flags.Arguments, "arg", "a", nil, "arguments to the buildpack template")

	AddHelpFlag(cmd, "create")

	cmd.AddCommand(BuildpackCreateInspect(logger))
	return cmd
}

func BuildpackCreateInspect(logger logging.Logger) *cobra.Command {
	flags := BuildpackCreateFlags{}
	cmd := &cobra.Command{
		Use:     "inspect",
		Short:   "inspect available buildpack templates",
		Example: "pack buildpack create inspect",
		Long:    "buildpack create inspect displays the available templates",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			s, err := scafall.NewScafall(flags.Template, scafall.WithSubPath(flags.SubPath))
			if err != nil {
				logger.Errorf("unable to get help for template: %s", err)
			} else {
				info, args, err := s.TemplateArguments()
				if err != nil {
					logger.Errorf("unable to get template arguments for template: %s", err)
				}
				logger.Info(info)
				for _, arg := range args {
					logger.Infof("\t%s", arg)
				}
			}
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.Template, "template", "t", CNBTemplates, "URL of the buildpack template git repository")
	cmd.Flags().StringVar(&flags.SubPath, "sub-path", "", "directory within template git repository used to generate the buildpack")
	return cmd
}
