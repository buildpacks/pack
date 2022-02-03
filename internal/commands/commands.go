package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/logging"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_pack_client.go github.com/buildpacks/pack/internal/commands PackClient
type PackClient interface {
	InspectBuilder(string, bool, ...client.BuilderInspectionModifier) (*client.BuilderInfo, error)
	InspectImage(string, bool) (*client.ImageInfo, error)
	Rebase(context.Context, client.RebaseOptions) error
	CreateBuilder(context.Context, client.CreateBuilderOptions) error
	NewBuildpack(context.Context, client.NewBuildpackOptions) error
	PackageBuildpack(ctx context.Context, opts client.PackageBuildpackOptions) error
	Build(context.Context, client.BuildOptions) error
	RegisterBuildpack(context.Context, client.RegisterBuildpackOptions) error
	YankBuildpack(client.YankBuildpackOptions) error
	InspectBuildpack(client.InspectBuildpackOptions) (*client.BuildpackInfo, error)
	PullBuildpack(context.Context, client.PullBuildpackOptions) error
	DownloadSBOM(name string, options client.DownloadSBOMOptions) error
}

func AddHelpFlag(cmd *cobra.Command, commandName string) {
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Help for '%s'", commandName))
}

func CreateCancellableContext() context.Context {
	signals := make(chan os.Signal, 1)
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
			if _, isSoftError := errors.Cause(err).(client.SoftError); !isSoftError {
				logger.Error(err.Error())
			}

			if _, isExpError := errors.Cause(err).(client.ExperimentError); isExpError {
				configPath, err := config.DefaultConfigPath()
				if err != nil {
					return err
				}
				enableExperimentalTip(logger, configPath)
			}
			return err
		}
		return nil
	}
}

func enableExperimentalTip(logger logging.Logger, configPath string) {
	logging.Tip(logger, "To enable experimental features, run `pack config experimental true` to add %s to %s.", style.Symbol("experimental = true"), style.Symbol(configPath))
}

func stringArrayHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order (comma-separated lists not accepted)", name)
}

func stringSliceHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order, or supply once by comma-separated list", name)
}

func getMirrors(config config.Config) map[string][]string {
	mirrors := map[string][]string{}
	for _, ri := range config.RunImages {
		mirrors[ri.Image] = ri.Mirrors
	}
	return mirrors
}

func isTrustedBuilder(cfg config.Config, builder string) bool {
	for _, trustedBuilder := range cfg.TrustedBuilders {
		if builder == trustedBuilder.Name {
			return true
		}
	}

	return isSuggestedBuilder(builder)
}

func deprecationWarning(logger logging.Logger, oldCmd, replacementCmd string) {
	logger.Warnf("Command %s has been deprecated, please use %s instead", style.Symbol("pack "+oldCmd), style.Symbol("pack "+replacementCmd))
}
