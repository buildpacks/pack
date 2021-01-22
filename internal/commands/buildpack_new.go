package commands

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

// BuildpackNewFlags define flags provided to the BuildpackCreate command
type BuildpackNewFlags struct {
	Path     string
	Stacks   []string
}

// BuildpackCreator creates buildpacks
type BuildpackCreator interface {
	NewBuildpack(ctx context.Context, options pack.NewBuildpackOptions) error
}

// BuildpackNew generates the scaffolding of a buildpack
func BuildpackNew(logger logging.Logger, client BuildpackCreator) *cobra.Command {
	var flags BuildpackNewFlags
	cmd := &cobra.Command{
		Use:     "new <name>",
		Short:   "Creates basic scaffolding of a buildpack.",
		Args:    cobra.ExactValidArgs(1),
		Example: "pack buildpack create my-buildpack",
		Long:    "buildpack create generates the basic scaffolding of a buildpack repository.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			path := flags.Path
			if len(path) == 0 {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				path = cwd
			}

			var stacks []dist.Stack
			for _, s := range flags.Stacks {
				stacks = append(stacks, dist.Stack{
					ID:     s,
					Mixins: []string{},
				})
			}

			id := args[0]
			if err := client.NewBuildpack(cmd.Context(), pack.NewBuildpackOptions{
				ID:     id,
				Path:   path,
				Stacks: stacks,
			}); err != nil {
				return err
			}

			logger.Infof("Successfully created %s", style.Symbol(id))
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Path to generate the buildpack")
	cmd.Flags().StringSliceVarP(&flags.Stacks, "stacks", "s", []string{"io.buildpacks.stacks.bionic"}, "Stack(s) this buildpack will be compatible with"+multiValueHelp("stack"))

	AddHelpFlag(cmd, "package")
	return cmd
}
