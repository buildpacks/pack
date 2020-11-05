package stack

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/logging"
)

func NewStackCmd(logger logging.Logger) *cobra.Command {
	command := cobra.Command{
		Use:   "stack",
		Short: "Displays stack information",
		RunE:  nil,
	}

	command.AddCommand(newSuggestCmd(logger))
	return &command
}
