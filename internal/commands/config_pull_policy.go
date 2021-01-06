package commands

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	pubcfg "github.com/buildpacks/pack/config"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

func ConfigPullPolicy(logger logging.Logger, cfg config.Config, cfgPath string) *cobra.Command {
	var unset bool

	cmd := &cobra.Command{
		Use:     "pull-policy <if-not-present | always | never>",
		Args:    cobra.MaximumNArgs(1),
		Short:   "List, set and unset the global pull policy used by other commands",
		Aliases: []string{"pull-policy"},
		Long: bpRegistryExplanation + "\n\nYou can use this command to list, set, and unset the default pull policy that will be used when working with containers:\n" +
			"* To list your pull policy, run `pack config pull-policy`.\n" +
			"* To set your pull policy, run `pack config pull-policy <pull-policy>`.\n" +
			"* To unset your pull policy, run `pack config pull-policy --unset`.\n" +
			fmt.Sprintf("Unsetting the pull policy will reset the policy to the default, which is always"),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			switch {
			case unset:
				if len(args) > 0 {
					return errors.Errorf("pull policy and --unset cannot be specified simultaneously")
				}
				oldPullPolicy := cfg.PullPolicy
				cfg.PullPolicy = ""
				if err := config.Write(cfg, cfgPath); err != nil {
					return errors.Wrapf(err, "writing config to %s", cfgPath)
				}

				pullPolicy, err := pubcfg.ParsePullPolicy(cfg.PullPolicy)
				if err != nil {
					return err
				}

				logger.Infof("Successfully unset pull policy %s", style.Symbol(oldPullPolicy))
				logger.Infof("Pull policy has been set to %s", style.Symbol(pullPolicy.String()))

			case len(args) == 0: // list
				pullPolicy, err := pubcfg.ParsePullPolicy(cfg.PullPolicy)
				if err != nil {
					return err
				}

				logger.Infof("The current pull policy is %s", style.Symbol(pullPolicy.String()))
			default: // set
				newPullPolicy := args[0]

				if newPullPolicy == cfg.PullPolicy {
					logger.Infof("Pull policy is already set to %s", newPullPolicy)
					return nil
				}

				pullPolicy, err := pubcfg.ParsePullPolicy(newPullPolicy)
				if err != nil {
					return err
				}

				cfg.PullPolicy = newPullPolicy
				if err := config.Write(cfg, cfgPath); err != nil {
					return errors.Wrapf(err, "writing config to %s", cfgPath)
				}

				logger.Infof("Successfully set %s as the pull policy", style.Symbol(pullPolicy.String()))
			}

			return nil
		}),
	}

	cmd.Flags().BoolVarP(&unset, "unset", "u", false, "Unset pull policy, and set it back to the default pull-policy, which is always")
	AddHelpFlag(cmd, "pull-policy")
	return cmd
}
