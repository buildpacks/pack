package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/internal/target"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

// ExtensionNewFlags define flags provided to the ExtensionNew command
type ExtensionNewFlags struct {
	API     string
	Path    string
	Stacks  []string
	Targets []string
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

			var targets []dist.Target
			if len(flags.Targets) == 0 && len(flags.Stacks) == 0 {
				targets = []dist.Target{{
					OS:   runtime.GOOS,
					Arch: runtime.GOARCH,
				}}
			} else {
				if targets, err = target.ParseTargets(flags.Targets, logger); err != nil {
					return err
				}
			}

			if err := creator.NewExtension(cmd.Context(), client.NewExtensionOptions{
				API:     flags.API,
				ID:      id,
				Path:    path,
				Targets: targets,
				Version: flags.Version,
			}); err != nil {
				return err
			}

			logger.Infof("Successfully created %s", style.Symbol(id))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.API, "api", "a", "0.9", "Buildpack API compatibility of the generated extension")
	cmd.Flags().StringVarP(&flags.Path, "path", "p", "", "Path to generate the extension")
	cmd.Flags().StringVarP(&flags.Version, "version", "V", "1.0.0", "Version of the generated extension")
	cmd.Flags().StringSliceVarP(&flags.Targets, "targets", "t", nil,
		`A list of platforms to target; these recorded in extension.toml. One can provide targets in the format [os][/arch][/arch-variant]:[distroname@osversion];[distroname@osversion]	
	- Example:  '--targets "linux/amd64" --targets "linux/arm64"'
	- Example (distribution version): '--targets "windows/amd64:windows-nano@10.0.19041.1415"'
	- Example (architecture with distributed versions): '--targets "linux/arm/v6:ubuntu@14.04"  --targets "linux/arm/v6:ubuntu@16.04"'	`)

	AddHelpFlag(cmd, "new")
	return cmd
}
