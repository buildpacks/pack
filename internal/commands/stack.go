package commands

import (
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
)

func NewStackCommand(logger logging.Logger) *cobra.Command {
	command := cobra.Command{
		Use:   "stack",
		Short: "(deprecated) Interact with stacks",
		Long:  "(Deprecated)\nStacks will continue to be supported through at least 2024 but are deprecated in favor of using BuildImages and RunImages directly. Please see our docs for more details- https://buildpacks.io/docs/concepts/components/stack",
		RunE:  nil,
	}

	command.AddCommand(stackSuggest(logger))
	return &command
}
