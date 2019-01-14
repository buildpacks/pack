package commands

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func ShowStacks(logger *logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stacks",
		Args:  cobra.NoArgs,
		Short: "Show information about available stacks",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			cfg, err := config.NewDefault()
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			w := tabwriter.NewWriter(&buf, 0, 0, 4, ' ', 0)
			// Note: Nop style is needed to keep color control characters from interfering with table formatting
			// See https://stackoverflow.com/questions/35398497/how-do-i-get-colors-to-work-with-golang-tabwriter
			fmt.Fprintf(w, "%s\t%s\t%s\n", style.Noop("Stack ID"), style.Noop("Build Image"), style.Noop("Run Image(s)"))
			fmt.Fprintf(w, "%s\t%s\t%s\n", style.Noop("--------"), style.Noop("-----------"), style.Noop("------------"))
			for _, stack := range cfg.Stacks {
				displayID := style.Key(stack.ID)
				if stack.ID == cfg.DefaultStackID {
					displayID = fmt.Sprintf("%s (default)", displayID)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", displayID, style.Noop(stack.BuildImage), style.Noop(strings.Join(stack.RunImages, ", ")))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			logger.Info(buf.String())
			return nil
		}),
	}
	AddHelpFlag(cmd, "stacks")
	return cmd
}
