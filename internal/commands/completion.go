package commands

import (
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

type CompletionFlags struct {
	Shell string
}

func CompletionCommand(logger logging.Logger) *cobra.Command {
	var flags CompletionFlags
	var completionCmd = &cobra.Command{
		Use:   "completion",
		Short: "Outputs completion script location",
		Long: `Generates bash completion script and outputs its location.

To configure your bash shell to load completions for each session, add the following to your '.bashrc' or '.bash_profile':

	. $(pack completion)
	`,
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			packHome, err := config.PackHome()
			if err != nil {
				return errors.Wrap(err, "getting pack home")
			}

			if err = config.MkdirAll(packHome); err != nil {
				return errors.Wrapf(err, "creating pack home: %s", packHome)
			}
			completionPath := filepath.Join(packHome, "completion")

			var flagErr error
			switch flags.Shell {
			case "bash":
				flagErr = cmd.Parent().GenBashCompletionFile(completionPath)
			case "zsh":
				flagErr = cmd.Parent().GenZshCompletionFile(completionPath)
			default:
				return errors.Errorf("%s is unsupported shell", flags.Shell)
			}

			if flagErr != nil {
				return err
			}

			logger.Infof("Completion File for %s is created", flags.Shell)
			logger.Info(completionPath)
			return nil
		}),
	}

	completionCmd.Flags().StringVarP(&flags.Shell, "shell", "s", "bash", "Generates completion file for [bash|zsh]")
	return completionCmd
}
