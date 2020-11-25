package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

type YankBuildpackFlags struct {
	BuildpackRegistry string
	Undo              bool
}

func YankBuildpack(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var flags YankBuildpackFlags

	cmd := &cobra.Command{
		Use:     "yank-buildpack <buildpack-id-and-version>",
		Args:    cobra.ExactArgs(1),
		Short:   PrependExperimental("Yank the buildpack from the registry"),
		Example: "pack yank-buildpack my-buildpack@0.0.1",
		RunE: LogError(logger, func(cmd *cobra.Command, args []string) error {
			buildpackIDVersion := args[0]

			registry, err := config.GetRegistry(cfg, flags.BuildpackRegistry)
			if err != nil {
				return err
			}
			id, version, err := parseIDVersion(buildpackIDVersion)
			if err != nil {
				return err
			}

			opts := pack.YankBuildpackOptions{
				ID:      id,
				Version: version,
				Type:    "github",
				URL:     registry.URL,
				Yank:    !flags.Undo,
			}

			if err := client.YankBuildpack(opts); err != nil {
				return err
			}
			logger.Infof("Successfully yanked %s", style.Symbol(buildpackIDVersion))
			return nil
		}),
	}
	cmd.Flags().StringVarP(&flags.BuildpackRegistry, "buildpack-registry", "r", "", "Buildpack Registry name")
	cmd.Flags().BoolVarP(&flags.Undo, "undo", "u", false, "undo previously yanked buildpack")
	AddHelpFlag(cmd, "yank-buildpack")

	return cmd
}

func parseIDVersion(buildpackIDVersion string) (string, string, error) {
	parts := strings.Split(buildpackIDVersion, "@")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid buildpack id@version %s", style.Symbol(buildpackIDVersion))
	}

	return parts[0], parts[1], nil
}
