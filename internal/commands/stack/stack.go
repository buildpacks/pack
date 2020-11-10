package stack

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/logging"
)

func Stack(logger logging.Logger) *cobra.Command {
	command := cobra.Command{
		Use:   "stack",
		Short: "Displays stack information",
		RunE:  nil,
	}

	command.AddCommand(suggest(logger))
	return &command
}
