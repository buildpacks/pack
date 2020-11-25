package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_pack_client.go github.com/buildpacks/pack/internal/commands PackClient
type PackClient interface {
	InspectBuilder(string, bool, ...pack.BuilderInspectionModifier) (*pack.BuilderInfo, error)
	InspectImage(string, bool) (*pack.ImageInfo, error)
	Rebase(context.Context, pack.RebaseOptions) error
	CreateBuilder(context.Context, pack.CreateBuilderOptions) error
	PackageBuildpack(ctx context.Context, opts pack.PackageBuildpackOptions) error
	Build(context.Context, pack.BuildOptions) error
	RegisterBuildpack(context.Context, pack.RegisterBuildpackOptions) error
	YankBuildpack(pack.YankBuildpackOptions) error
	InspectBuildpack(pack.InspectBuildpackOptions) (*pack.BuildpackInfo, error)
	PullBuildpack(context.Context, pack.PullBuildpackOptions) error
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

func LogError(logger logging.Logger, f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		err := f(cmd, args)
		if err != nil {
			if _, isSoftError := errors.Cause(err).(pack.SoftError); !isSoftError {
				logger.Error(err.Error())
			}

			if _, isExpError := errors.Cause(err).(pack.ExperimentError); isExpError {
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
	logging.Tip(logger, "To enable experimental features, add %s to %s.", style.Symbol("experimental = true"), style.Symbol(configPath))
}

func MultiValueHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order,\n  or supply once by comma-separated list", name)
}

func PrependExperimental(short string) string {
	return fmt.Sprintf("(%s) %s", style.Warn("experimental"), short)
}

func GetMirrors(config config.Config) map[string][]string {
	mirrors := map[string][]string{}
	for _, ri := range config.RunImages {
		mirrors[ri.Image] = ri.Mirrors
	}
	return mirrors
}

func IsTrustedBuilder(cfg config.Config, builder string) bool {
	for _, trustedBuilder := range cfg.TrustedBuilders {
		if builder == trustedBuilder.Name {
			return true
		}
	}

	return IsSuggestedBuilder(builder)
}

func IsSuggestedBuilder(builder string) bool {
	for _, sugBuilder := range SuggestedBuilders {
		if builder == sugBuilder.Image {
			return true
		}
	}

	return false
}

type SuggestedBuilder struct {
	Vendor             string
	Image              string
	DefaultDescription string
}

var SuggestedBuilders = []SuggestedBuilder{
	{
		Vendor:             "Google",
		Image:              "gcr.io/buildpacks/builder:v1",
		DefaultDescription: "GCP Builder for all runtimes",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/buildpacks:18",
		DefaultDescription: "heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder:base",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js, Golang, & .NET Core",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder:full",
		DefaultDescription: "Larger base image with buildpacks for Java, Node.js, Golang, .NET Core, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "paketobuildpacks/builder:tiny",
		DefaultDescription: "Tiny base image (bionic build image, distroless run image) with buildpacks for Golang",
	},
}

func SuggestSettingBuilder(logger logging.Logger, inspector BuilderInspector) {
	logger.Info("Please select a default builder with:")
	logger.Info("")
	logger.Info("\tpack set-default-builder <builder-image>")
	logger.Info("")
	SuggestBuilders(logger, inspector)
}

type BuilderInspector interface {
	InspectBuilder(name string, daemon bool, modifiers ...pack.BuilderInspectionModifier) (*pack.BuilderInfo, error)
}

func SuggestBuilders(logger logging.Logger, client BuilderInspector) {
	WriteSuggestedBuilder(logger, client, SuggestedBuilders)
}

func WriteSuggestedBuilder(logger logging.Logger, inspector BuilderInspector, builders []SuggestedBuilder) {
	sort.Slice(builders, func(i, j int) bool {
		if builders[i].Vendor == builders[j].Vendor {
			return builders[i].Image < builders[j].Image
		}

		return builders[i].Vendor < builders[j].Vendor
	})

	logger.Info("Suggested builders:")

	// Fetch descriptions concurrently.
	descriptions := make([]string, len(builders))

	var wg sync.WaitGroup
	for i, builder := range builders {
		wg.Add(1)

		go func(i int, builder SuggestedBuilder) {
			descriptions[i] = getBuilderDescription(builder, inspector)
			wg.Done()
		}(i, builder)
	}
	wg.Wait()

	tw := tabwriter.NewWriter(logger.Writer(), 10, 10, 5, ' ', tabwriter.TabIndent)
	for i, builder := range builders {
		fmt.Fprintf(tw, "\t%s:\t%s\t%s\t\n", builder.Vendor, style.Symbol(builder.Image), descriptions[i])
	}
	fmt.Fprintln(tw)

	logging.Tip(logger, "Learn more about a specific builder with:")
	logger.Info("\tpack inspect-builder <builder-image>")
}

func getBuilderDescription(builder SuggestedBuilder, inspector BuilderInspector) string {
	info, err := inspector.InspectBuilder(builder.Image, false)
	if err == nil && info.Description != "" {
		return info.Description
	}

	return builder.DefaultDescription
}
