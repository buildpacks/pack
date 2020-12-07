package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

// Deprecated: Use yank instead
func YankBuildpack(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var flags BuildpackYankFlags

	cmd := &cobra.Command{
		Use:     "yank-buildpack <buildpack-id-and-version>",
		Hidden:  true,
		Args:    cobra.ExactArgs(1),
		Short:   prependExperimental("Yank the buildpack from the registry"),
		Example: "pack yank-buildpack my-buildpack@0.0.1",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			deprecationWarning(logger, "yank-buildpack", "buildpack yank")
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
