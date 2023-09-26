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

// BuildpackNewFlags define flags provided to the BuildpackNew command
type BuildpackNewFlags struct {
	API     string
	Path    string
	// Deprecated: use `targets` instead
	Stacks  []string
	Targets dist.Targets
	Version string
}

// BuildpackCreator creates buildpacks
type BuildpackCreator interface {
	NewBuildpack(ctx context.Context, options client.NewBuildpackOptions) error
}

// BuildpackNew generates the scaffolding of a buildpack
func BuildpackNew(logger logging.Logger, creator BuildpackCreator) *cobra.Command {
	var flags BuildpackNewFlags
	cmd := &cobra.Command{
		Use:     "new <id>",
		Short:   "Creates basic scaffolding of a buildpack.",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Example: "pack buildpack new sample/my-buildpack",
		Long:    "buildpack new generates the basic scaffolding of a buildpack repository. It creates a new directory `name` in the current directory (or at `path`, if passed as a flag), and initializes a buildpack.toml, and two executable bash scripts, `bin/detect` and `bin/build`. ",
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

			var targets dist.Targets
			for _, t := range flags.Targets {
				targets = append(targets, dist.Target{
					OS: t.OS,
					Arch: t.Arch,
					ArchVariant: t.ArchVariant,
					Distributions: t.Distributions,
				})
			}

			if err := creator.NewBuildpack(cmd.Context(), client.NewBuildpackOptions{
				API:     flags.API,
				ID:      id,
				Path:    path,
				Stacks:  stacks,
				Targets: targets,
				Version: flags.Version,
			}); err != nil {
				return err
			}

			logger.Infof("Successfully created %s", style.Symbol(id))
			return nil
		}),
	}

	cmd.Flags().StringVarP(&flags.API, "api", "a", "0.8", "Buildpack API compatibility of the generated buildpack")
	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Path to generate the buildpack")
	cmd.Flags().StringVarP(&flags.Version, "version", "V", "1.0.0", "Version of the generated buildpack")
	cmd.Flags().StringSliceVarP(&flags.Stacks, "stacks", "s", []string{"io.buildpacks.stacks.jammy"}, "Stack(s) this buildpack will be compatible with"+stringSliceHelp("stack"))
	cmd.Flags().MarkDeprecated("stacks", "stacks is deprecated in the favor of `targets`")
	cmd.Flags().Var(&flags.Targets, "targets",
		`Targets is a list of platforms that you wish to support. one can provide target platforms in format [os][/arch][/variant]:[name@osversion]
- Base case for two different architectures :  '--targets "linux/amd64" --targets "linux/arm64"'
- case for distribution versions: '--targets "windows/amd64:windows-nano@10.0.19041.1415"'
- case for different architecture with distrubuted versions : '--targets "linux/arm/v6:ubuntu@14.04"  --targets "linux/arm/v6:ubuntu@16.04"'
    - If no name is provided, a random name will be generated.
`)

	AddHelpFlag(cmd, "new")
	return cmd
}
