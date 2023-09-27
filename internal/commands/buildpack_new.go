package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

// BuildpackNewFlags define flags provided to the BuildpackNew command
type BuildpackNewFlags struct {
	API  string
	Path string
	// Deprecated: Stacks are deprecated
	Stacks  []string
	Targets []string
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

			var targets []dist.Target
			for _, t := range flags.Targets {
				var distroMap []dist.Distribution
				var nonDistro []string
				var distros []string
				target := strings.Split(t, ":")
				if i, e := getSliceAt[string](target, 0); e != nil {
					logger.Errorf("invalid target %s, atleast one of [os][/arch][/archVariant] must be specified", t)
				} else {
					nonDistro = strings.Split(i, "/")
				}
				if i, e := getSliceAt[string](target, 1); e != nil {
					logger.Errorf("invalid target %s, atleast one of [name][version] must be specified", t)
				} else {
					distros = strings.Split(i, ";")
				}
				for _, d := range distros {
					distro := strings.Split(d, "@")
					if l := len(distro); l <= 0 {
						logger.Error("distro is nil!")
					} else if l == 1 {
						logger.Warnf("forgot to specify version for distro %s ?", distro[0])
					}
					distroMap = append(distroMap, dist.Distribution{
						Name:     distro[0],
						Versions: distro[1:],
					})
				}
				os, _ := getSliceAt[string](nonDistro, 0)
				arch, _ := getSliceAt[string](nonDistro, 1)
				variant, _ := getSliceAt[string](nonDistro, 2)
				targets = append(targets, dist.Target{
					OS:            os,
					Arch:          arch,
					ArchVariant:   variant,
					Distributions: distroMap,
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
	cmd.Flags().StringSliceVarP(&flags.Stacks, "stacks", "s", []string{}, "Stack(s) this buildpack will be compatible with"+stringSliceHelp("stack"))
	cmd.Flags().MarkDeprecated("stacks", "")
	cmd.Flags().StringSliceVarP(&flags.Targets, "targets", "t", []string{"/"},
		`Targets are the list platforms that one targeting, these are generated as part of scaffolding inside buildpack.toml file. one can provide target platforms in format [os][/arch][/variant]:[distroname@osversion@anotherversion];[distroname@osversion]
	- Base case for two different architectures :  '--targets "linux/amd64" --targets "linux/arm64"'
	- case for distribution version: '--targets "windows/amd64:windows-nano@10.0.19041.1415"'
	- case for different architecture with distributed versions : '--targets "linux/arm/v6:ubuntu@14.04"  --targets "linux/arm/v6:ubuntu@16.04"'
	`)

	AddHelpFlag(cmd, "new")
	return cmd
}

func getSliceAt[T interface{}](slice []T, index int) (T, error) {
	if index < 0 || index >= len(slice) {
		var r T
		return r, errors.Errorf("index out of bound, cannot access item at index %d of slice with length %d", index, len(slice))
	}

	return slice[index], nil
}
