package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
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
type ExtensionCreator interface {
	NewExtension(ctx context.Context, options client.NewExtensionOptions) error
}

// ExtensionNew generates the scaffolding of an extension
func ExtensionNew(logger logging.Logger, creator ExtensionCreator) *cobra.Command {
	var flags ExtensionNewFlags
	cmd := &cobra.Command{
		Use:     "new <id>",
		Short:   "Creates basic scaffolding of an extension",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Example: "pack extension new <example-extension>",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			id := args[0]
			idParts := strings.Split(id, "/")
			dirName := idParts[len(idParts)-1]

			var path string
			if len(flags.Path) == 0 {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				path = filepath.Join(cwd, dirName)
			} else {
				path = flags.Path
			}

			_, err := os.Stat(path)
			if !os.IsNotExist(err) {
				return fmt.Errorf("directory %s exists", style.Symbol(path))
			}

			var stacks []dist.Stack
			for _, s := range flags.Stacks {
				stacks = append(stacks, dist.Stack{
					ID:     s,
					Mixins: []string{},
				})
			}

			if err := creator.NewExtension(cmd.Context(), client.NewExtensionOptions{
				API:     flags.API,
				ID:      id,
				Path:    path,
				Stacks:  stacks,
				Version: flags.Version,
			}); err != nil {
				return err
			}

			logger.Infof("Successfully created %s", style.Symbol(id))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.API, "api", "a", "0.8", "Extension API compatibility of the generated buildpack")
	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Path to generate the extension")
	cmd.Flags().StringVarP(&flags.Version, "version", "V", "1.0.0", "Version of the generated extension")
	cmd.Flags().StringSliceVarP(&flags.Stacks, "stacks", "s", nil, "Stack(s) this buildpack will be compatible with"+stringSliceHelp("stack"))
	cmd.Flags().MarkDeprecated("stacks", "prefer `--targets` instead: https://github.com/buildpacks/rfcs/blob/main/text/0096-remove-stacks-mixins.md")
	
	AddHelpFlag(cmd, "new")
	return cmd
}
