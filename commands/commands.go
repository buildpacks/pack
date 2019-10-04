package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

//go:generate mockgen -package mocks -destination mocks/pack_client.go github.com/buildpack/pack/commands PackClient
type PackClient interface {
	InspectBuilder(string, bool) (*pack.BuilderInfo, error)
	Rebase(context.Context, pack.RebaseOptions) error
	CreateBuilder(context.Context, pack.CreateBuilderOptions) error
	Build(context.Context, pack.BuildOptions) error
}

type suggestedBuilder struct {
	Name               string
	Image              string
	DefaultDescription string
}

var suggestedBuilders = []suggestedBuilder{
	{
		Name:               "Cloud Foundry",
		Image:              "cloudfoundry/cnb:bionic",
		DefaultDescription: "Small base image with Java & Node.js",
	},
	{
		Name:               "Cloud Foundry",
		Image:              "cloudfoundry/cnb:cflinuxfs3",
		DefaultDescription: "Larger base image with Java, Node.js & Python",
	},
	{
		Name:               "Heroku",
		Image:              "heroku/buildpacks:18",
		DefaultDescription: "heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
	},
}

type suggestedStack struct {
	ID          string
	Description string
	Maintainer  string
	BuildImage  string
	RunImage    string
}

var suggestedStacks = []suggestedStack{
	{
		ID:          "heroku-18",
		Description: "The official Heroku stack based on Ubuntu 18.04",
		Maintainer:  "Heroku",
		BuildImage:  "heroku/pack:18-build",
		RunImage:    "heroku/pack:18",
	},
	{
		ID:          "io.buildpacks.stacks.bionic",
		Description: "A minimal Cloud Foundry stack based on Ubuntu 18.04",
		Maintainer:  "Cloud Foundry",
		BuildImage:  "cloudfoundry/build:base-cnb",
		RunImage:    "cloudfoundry/run:base-cnb",
	},
	{
		ID:          "org.cloudfoundry.stacks.cflinuxfs3",
		Description: "A large Cloud Foundry stack based on Ubuntu 18.04",
		Maintainer:  "Cloud Foundry",
		BuildImage:  "cloudfoundry/build:full-cnb",
		RunImage:    "cloudfoundry/run:full-cnb",
	},
	{
		ID:          "org.cloudfoundry.stacks.tiny",
		Description: "A tiny Cloud Foundry stack based on Ubuntu 18.04, similar to distroless",
		Maintainer:  "Cloud Foundry",
		BuildImage:  "cloudfoundry/build:tiny-cnb",
		RunImage:    "cloudfoundry/run:tiny-cnb",
	},
}

func AddHelpFlag(cmd *cobra.Command, commandName string) {
	cmd.Flags().BoolP("help", "h", false, fmt.Sprintf("Help for '%s'", commandName))
}

func logError(logger logging.Logger, f func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		err := f(cmd, args)
		if err != nil {
			if !IsSoftError(err) {
				logger.Error(err.Error())
			}
			return err
		}
		return nil
	}
}

func multiValueHelp(name string) string {
	return fmt.Sprintf("\nRepeat for each %s in order,\n  or supply once by comma-separated list", name)
}

func createCancellableContext() context.Context {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-signals
		cancel()
	}()

	return ctx
}

func getMirrors(config config.Config) map[string][]string {
	mirrors := map[string][]string{}
	for _, ri := range config.RunImages {
		mirrors[ri.Image] = ri.Mirrors
	}
	return mirrors
}

func suggestSettingBuilder(logger logging.Logger, client PackClient) {
	logger.Info("Please select a default builder with:")
	logger.Info("")
	logger.Info("\tpack set-default-builder <builder image>")
	logger.Info("")
	suggestBuilders(logger, client)
}

func suggestBuilders(logger logging.Logger, client PackClient) {
	logger.Info("Suggested builders:")
	tw := tabwriter.NewWriter(logger.Writer(), 10, 10, 5, ' ', tabwriter.TabIndent)
	for _, builder := range suggestedBuilders {
		fmt.Fprintf(tw, "\t%s:\t%s\t%s\t\n", builder.Name, style.Symbol(builder.Image), getBuilderDescription(builder, client))
	}
	fmt.Fprintln(tw)

	logging.Tip(logger, "Learn more about a specific builder with:\n")
	logger.Info("\tpack inspect-builder [builder image]")
}

func getBuilderDescription(builder suggestedBuilder, client PackClient) string {
	info, err := client.InspectBuilder(builder.Image, false)
	if err == nil && info.Description != "" {
		return info.Description
	}

	return builder.DefaultDescription
}

func suggestStacks(log logging.Logger) {
	tmpl := template.Must(template.New("").Parse(`
Stacks maintained by the community:
{{- range . }}

    Stack ID: {{ .ID }}
    Description: {{ .Description }}
    Maintainer: {{ .Maintainer }}
    Build Image: {{ .BuildImage }}
    Run Image: {{ .RunImage }}
{{- end }}
`))
	tmpl.Execute(log.Writer(), suggestedStacks)
}
