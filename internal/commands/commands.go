package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_pack_client.go github.com/buildpacks/pack/internal/commands PackClient
type PackClient interface {
	InspectBuilder(string, bool) (*pack.BuilderInfo, error)
	InspectImage(string, bool) (*pack.ImageInfo, error)
	Rebase(context.Context, pack.RebaseOptions) error
	CreateBuilder(context.Context, pack.CreateBuilderOptions) error
	PackageBuildpack(ctx context.Context, opts pack.PackageBuildpackOptions) error
	Build(context.Context, pack.BuildOptions) error
}

func AddHelpFlag(cmd *cobra.Command, commandName string) {
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Help for '%s'", commandName))
}

func CreateCancellableContext() context.Context {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-signals
		cancel()
	}()

	return ctx
}

func logError(logger logging.Logger, f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		err := f(cmd, args)
		if err != nil {
			if _, isSoftError := err.(SoftError); !isSoftError {
				logger.Error(err.Error())
			}
			if ee, isExpError := err.(ExperimentError); isExpError {
				configPath, err := config.DefaultConfigPath()
				if err != nil {
					return err
				}
				ee.Tip(logger, configPath)
			}
			return err
		}
		return nil
	}
}

func multiValueHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order,\n  or supply once by comma-separated list", name)
}

func getMirrors(config config.Config) map[string][]string {
	mirrors := map[string][]string{}
	for _, ri := range config.RunImages {
		mirrors[ri.Image] = ri.Mirrors
	}
	return mirrors
}
