package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

func NewStackCommand(logger logging.Logger) *cobra.Command {
	command := cobra.Command{
		Use:   "stack",
		Short: "Interact with stacks",
		RunE:  nil,
	}

	command.AddCommand(stackSuggest(logger))
	return &command
}
