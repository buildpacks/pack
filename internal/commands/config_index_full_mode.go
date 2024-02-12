package commands

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/logging"
)

func ConfigImageIndexFullMode(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index-full-mode [<true | false>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "List and set the current 'index-full-mode' value from the config",
		Long: "ImageIndex FullMode features in pack are gated, and require you adding setting `index-full-mode=true` to the Pack Config, either manually, or using this command.\n\n" +
			"* Running `pack config index-full-mode` prints whether ImageIndexFullMode features are currently enabled.\n" +
			"* Running `pack config index-full-mode <true | false>` enables or disables ImageIndexFullMode features.",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 0:
				if cfg.ImageIndexFullMode {
					logger.Infof("ImageIndexFullMode features are enabled! To turn them off, run `pack config index-full-mode false`")
				} else {
					logger.Info("ImageIndexFullMode features aren't currently enabled. To enable them, run `pack config index-full-mode true`")
				}
			default:
				val, err := strconv.ParseBool(args[0])
				if err != nil {
					return errors.Wrapf(err, "invalid value %s provided", style.Symbol(args[0]))
				}
				cfg.ImageIndexFullMode = val

				if err = config.Write(cfg, cfgPath); err != nil {
					return errors.Wrap(err, "writing to config")
				}

				if cfg.ImageIndexFullMode {
					logger.Info("ImageIndexFullMode features enabled!")
				} else {
					logger.Info("ImageIndexFullMode features disabled")
				}
			}

			return nil
		}),
	}

	AddHelpFlag(cmd, "index-full-mode")
	return cmd
}
