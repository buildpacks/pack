package commands

import (
	"context"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	scafall "github.com/buildpacks/scafall/pkg"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

// ExtensionNewFlags define flags provided to the ExtensionNew command
type ExtensionNewFlags struct {
	API     string
	Path    string
	Stacks  []string
	Version string
}

// extensioncreator type to be added here and argument also to be added in the function
type ExtensionCreateFlag struct {
	Arguments map[string]string
	Template  string
	SubPath   string
}

type ExtensionCreateCreator interface {
	CreateExtension(ctx context.Context, options client.CreateExtensionOptions) error
}

const (
	CNBTemplates = "https://github.com/buildpacks/templates"
)

// ExtensionNew generates the scaffolding of an extension
func ExtensionNew(logger logging.Logger, creator ExtensionCreateCreator) *cobra.Command {
	flags := ExtensionCreateFlag{}
	cmd := &cobra.Command{
		Use:     "new <id>",
		Short:   "Creates basic scaffolding of an extension",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Example: "pack extension new <example-extension>",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if err := creator.CreateExtension(cmd.Context(), client.CreateExtensionOptions{
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

	// flags will go here
	cmd.Flags().StringVarP(&flags.Template, "template", "t", CNBTemplates, "URL of the extension template git repository")
	cmd.Flags().StringVar(&flags.SubPath, "sub-path", "", "directory within template git repository used to generate the extension")
	cmd.Flags().StringToStringVarP(&flags.Arguments, "arg", "a", nil, "arguments to the extension template")

	cmd.SetHelpFunc(func(*cobra.Command, []string) {
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
	})
	return cmd

	AddHelpFlag(cmd, "new")
	return cmd
}
