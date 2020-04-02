package commands

import (
	"fmt"
	"sort"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type suggestedBuilder struct {
	Name               string
	Image              string
	DefaultDescription string
}

var suggestedBuilders = []suggestedBuilder{
	{
		Name:               "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:bionic",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js, Golang, & .NET Core",
	},
	{
		Name:               "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:cflinuxfs3",
		DefaultDescription: "Larger base image with buildpacks for Java, Node.js, Golang, .NET Core, & PHP",
	},
	{
		Name:               "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:tiny",
		DefaultDescription: "Tiny base image (bionic build image, distroless run image) with buildpacks for Golang",
	},
	{
		Name:               "Heroku",
		Image:              "heroku/buildpacks:18",
		DefaultDescription: "heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
	},
}

func SuggestBuilders(logger logging.Logger, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-builders",
		Short: "Display list of recommended builders",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, s []string) {
			suggestBuilders(logger, client)
		},
	}

	AddHelpFlag(cmd, "suggest-builders")
	return cmd
}

func suggestSettingBuilder(logger logging.Logger, client PackClient) {
	logger.Info("Please select a default builder with:")
	logger.Info("")
	logger.Info("\tpack set-default-builder <builder image>")
	logger.Info("")
	suggestBuilders(logger, client)
}

func suggestBuilders(logger logging.Logger, client PackClient) {
	sort.Slice(suggestedBuilders, func(i, j int) bool { return suggestedBuilders[i].Image < suggestedBuilders[j].Image })

	logger.Info("Suggested builders:")

	// Fetch descriptions concurrently.
	descriptions := make([]string, len(suggestedBuilders))

	var wg sync.WaitGroup
	for i, builder := range suggestedBuilders {
		wg.Add(1)

		go func(i int, builder suggestedBuilder) {
			descriptions[i] = getBuilderDescription(builder, client)
			wg.Done()
		}(i, builder)
	}
	wg.Wait()

	tw := tabwriter.NewWriter(logger.Writer(), 10, 10, 5, ' ', tabwriter.TabIndent)
	for i, builder := range suggestedBuilders {
		fmt.Fprintf(tw, "\t%s:\t%s\t%s\t\n", builder.Name, style.Symbol(builder.Image), descriptions[i])
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
