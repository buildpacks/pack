package commands

import (
	"fmt"

	"github.com/buildpack/lifecycle/image"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack/logging"
)

//go:generate mockgen -package mocks -destination mocks/image_factory.go github.com/buildpack/pack/commands ImageFactory
type ImageFactory interface {
	NewLocal(string, bool) (image.Image, error)
	NewRemote(string) (image.Image, error)
}

// TODO: Check if most recent cobra version fixed bug in help strings. It was not always capitalizing the first
// letter in the help string. If it's fixed, we can remove this.
func AddHelpFlag(cmd *cobra.Command, commandName string) {
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Help for '%s'", commandName))
}

func logError(logger *logging.Logger, f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		err := f(cmd, args)
		if err != nil {
			logger.Error(err.Error())
			return err
		}
		return nil
	}
}

func multiValueHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order,\n  or supply once by comma-separated list", name)
}
