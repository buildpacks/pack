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

// BuildpackCreateFlags define flags provided to the BuildpackCreate command
type BuildpackCreateFlags struct {
	Path     string
	Language string
	Stacks   []string
}

// BuildpackCreator creates buildpacks
type BuildpackCreator interface {
	CreateBuildpack(ctx context.Context, options pack.CreateBuildpackOptions) error
}

// BuildpackCreate generates the scaffolding of a buildpack
func BuildpackCreate(logger logging.Logger, client BuildpackCreator) *cobra.Command {
	var flags BuildpackCreateFlags
	cmd := &cobra.Command{
		Use:     "create <name>",
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
			if err := client.CreateBuildpack(cmd.Context(), pack.CreateBuildpackOptions{
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
	cmd.Flags().StringSliceVarP(&flags.Stacks, "stacks", "s", []string{"io.buildpacks.stacks.bionic"}, "Stack(s) this buildpack will be compatible with")

	AddHelpFlag(cmd, "package")
	return cmd
}
