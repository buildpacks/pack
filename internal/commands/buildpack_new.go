package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
				target, nonDistro, distros, err := getTarget(t)
				if err != nil {
					logger.Error(err.Error())
				}
				if i, e := getSliceAt[string](target, 1); e == nil {
					distros = strings.Split(i, ";")
				}
				for _, d := range distros {
					distro := strings.Split(d, "@")
					if l := len(distro); l <= 0 {
						return errors.Errorf("distro is nil!")
					} else if l == 1 {
						logger.Warnf("forgot to specify version for distro %s ?", distro[0])
					}
					distroMap = append(distroMap, dist.Distribution{
						Name:     distro[0],
						Versions: distro[1:],
					})
				}
				os, arch, variant, err := getPlatform(nonDistro)
				if err != nil {
					logger.Error(err.Error())
				}
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
	cmd.Flags().MarkDeprecated("stacks", "prefer `--targets` instead: https://github.com/buildpacks/rfcs/blob/main/text/0096-remove-stacks-mixins.md")
	cmd.PersistentFlags().StringSliceVarP(&flags.Targets, "targets", "t", []string{runtime.GOOS + "/" + runtime.GOARCH},
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

var GOOSArch = map[string][]string{
	"aix":       {"ppc64"},
	"android":   {"386", "amd64", "arm", "arm64"},
	"darwin":    {"amd64", "arm64"},
	"dragonfly": {"amd64"},
	"freebsd":   {"386", "amd64", "arm"},
	"illumos":   {"amd64"},
	"ios":       {"arm64"},
	"js":        {"wasm"},
	"linux":     {"386", "amd64", "arm", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x"},
	"netbsd":    {"386", "amd64", "arm"},
	"openbsd":   {"386", "amd64", "arm", "arm64"},
	"plan9":     {"386", "amd64", "arm"},
	"solaris":   {"amd64"},
	"wasip1":    {"wasm"},
	"windows":   {"386", "amd64", "arm", "arm64"},
}

var GOArchVariant = map[string][]string{
	"386":      {"softfloat", "sse2"},
	"arm":      {"v5", "v6", "v7"},
	"amd64":    {"v1", "v2", "v3", "v4"},
	"mips":     {"hardfloat", "softfloat"},
	"mipsle":   {"hardfloat", "softfloat"},
	"mips64":   {"hardfloat", "softfloat"},
	"mips64le": {"hardfloat", "softfloat"},
	"ppc64":    {"power8", "power9"},
	"ppc64le":  {"power8", "power9"},
	"wasm":     {"satconv", "signext"},
}

func isOS(os string) bool {
	return GOOSArch[os] != nil
}

func supportsArch(os string, arch string) bool {
	if isOS(os) {
		var supported bool
		for _, s := range GOOSArch[os] {
			if s == arch {
				supported = true
				break
			}
		}
		return supported
	}
	return false
}

func supportsVariant(arch string, variant string) bool {
	if variant == "" || len(variant) == 0 {
		return true
	}
	var supported bool
	for _, s := range GOArchVariant[arch] {
		if s == variant {
			supported = true
			break
		}
	}
	return supported
}

func getTarget(t string) ([]string, []string, []string, error) {
	var nonDistro, distro []string
	target := strings.Split(t, ":")
	if i, e := getSliceAt[string](target, 0); e != nil {
		return target, nonDistro, distro, errors.Errorf("invalid target %s, atleast one of [os][/arch][/archVariant] must be specified", t)
	} else {
		nonDistro = strings.Split(i, "/")
	}
	return target, nonDistro, distro, nil
}

func getPlatform(t []string) (string, string, string, error) {
	os, _ := getSliceAt[string](t, 0)
	arch, _ := getSliceAt[string](t, 1)
	variant, _ := getSliceAt[string](t, 2)
	if !isOS(os) || !supportsArch(os, arch) || !supportsVariant(arch, variant) {
		return os, arch, variant, errors.Errorf("unknown target: %s", style.Symbol(strings.Join(t, "/")))
	}
	return os, arch, variant, nil
}
