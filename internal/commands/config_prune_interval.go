package commands

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/image"
	"github.com/buildpacks/pack/pkg/logging"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func ConfigPruneInterval(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	var unset bool
	var intervalRegex = regexp.MustCompile(`^(\d+d)?(\d+h)?(\d+m)?$`)

	cmd := &cobra.Command{
		Use:   "prune-interval",
		Args:  cobra.MaximumNArgs(1),
		Short: "List, set, and unset the global pruning interval used for cleaning up outdated image entries from $HOME/.pack/image.json file",
		Long: "You can use this command to list, set, and unset the default pruning interval for cleaning up unused images:\n" +
			"* To list your current pruning interval, run `pack config prune-interval`.\n" +
			"* To set a new pruning interval, run `pack config prune-interval <interval>` where <interval> is a duration string (e.g., '7d' for 7 days).\n" +
			"* To unset the pruning interval, run `pack config prune-interval --unset`.\n" +
			fmt.Sprintf("Unsetting the pruning interval will reset the interval to the default, which is %s.", style.Symbol("7 days")),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			switch {
			case unset:
				if len(args) > 0 {
					return errors.Errorf("prune inteval and --unset cannot be specified simultaneously")
				}
				imageJSON, err := image.ReadImageJSON(logger)
				if err != nil {
					return err
				}
				oldPruneInterval := imageJSON.Interval.PruningIinterval
				imageJSON.Interval.PruningIinterval = "7d"

				updatedJSON, err := json.MarshalIndent(imageJSON, "", "    ")
				if err != nil {
					return errors.Wrap(err, "failed to marshal updated records")
				}
				err = image.WriteFile(updatedJSON)
				if err != nil {
					return err
				}
				logger.Infof("Successfully unset pruning interval %s", style.Symbol(oldPruneInterval))
				logger.Infof("Pruning interval has been set to %s", style.Symbol(imageJSON.Interval.PruningIinterval))
			case len(args) == 0: // list
				imageJSON, err := image.ReadImageJSON(logger)
				if err != nil {
					return err
				}
				pruneInterval := imageJSON.Interval.PruningIinterval
				if err != nil {
					return err
				}

				logger.Infof("The current prune interval is %s", style.Symbol(pruneInterval))
			default: // set
				newPruneInterval := args[0]

				imageJSON, err := image.ReadImageJSON(logger)
				if err != nil {
					return err
				}
				pruneInterval := imageJSON.Interval.PruningIinterval
				if err != nil {
					return err
				}

				if newPruneInterval == pruneInterval {
					logger.Infof("Prune Interval is already set to %s", style.Symbol(newPruneInterval))
					return nil
				}

				matches := intervalRegex.FindStringSubmatch(newPruneInterval)
				if len(matches) == 0 {
					return errors.Errorf("invalid interval format: %s", newPruneInterval)
				}

				imageJSON.Interval.PruningIinterval = newPruneInterval
				updatedJSON, err := json.MarshalIndent(imageJSON, "", "    ")
				if err != nil {
					return errors.Wrap(err, "failed to marshal updated records")
				}
				err = image.WriteFile(updatedJSON)
				if err != nil {
					return err
				}

				logger.Infof("Successfully set %s as the pruning interval", style.Symbol(imageJSON.Interval.PruningIinterval))
			}

			return nil
		}),
	}
	cmd.Flags().BoolVarP(&unset, "unset", "u", false, "Unset prune interval, and set it back to the default prune-interval, which is "+style.Symbol("7d"))
	AddHelpFlag(cmd, "prune-interval")
	return cmd
}
