package commands

import (
	"os"

	"github.com/spf13/cobra"
)

func AutoCompletionCommand() *cobra.Command {
	var completionCmd = &cobra.Command{
		Use:   "completion",
		Short: "Generates bash completion scripts",
		Long: `To load completion run

	. <(pack completion)

	To configure your bash shell to load completions for each session add to your bashrc

	# ~/.bashrc or ~/.profile
	. <(pack completion)
	`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Parent().GenBashCompletion(os.Stdout)
		},
	}
	return completionCmd
}
