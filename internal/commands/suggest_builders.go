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

type SuggestedBuilder struct {
	Vendor             string
	Image              string
	DefaultDescription string
}

var suggestedBuilders = []SuggestedBuilder{
	{
		Vendor:             "Google",
		Image:              "gcr.io/buildpacks/builder",
		DefaultDescription: "GCP Builder for all runtimes",
	},
	{
		Vendor:             "Heroku",
		Image:              "heroku/buildpacks:18",
		DefaultDescription: "heroku-18 base image with buildpacks for Ruby, Java, Node.js, Python, Golang, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:base",
		DefaultDescription: "Small base image with buildpacks for Java, Node.js, Golang, & .NET Core",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:full-cf",
		DefaultDescription: "Larger base image with buildpacks for Java, Node.js, Golang, .NET Core, & PHP",
	},
	{
		Vendor:             "Paketo Buildpacks",
		Image:              "gcr.io/paketo-buildpacks/builder:tiny",
		DefaultDescription: "Tiny base image (bionic build image, distroless run image) with buildpacks for Golang",
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
	logger.Info("\tpack set-default-builder <builder-image>")
	logger.Info("")
	suggestBuilders(logger, client)
}

func suggestBuilders(logger logging.Logger, client PackClient) {
	WriteSuggestedBuilder(logger, client, suggestedBuilders)
}

func WriteSuggestedBuilder(logger logging.Logger, client PackClient, builders []SuggestedBuilder) {
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
			descriptions[i] = getBuilderDescription(builder, client)
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

func getBuilderDescription(builder SuggestedBuilder, client PackClient) string {
	info, err := client.InspectBuilder(builder.Image, false)
	if err == nil && info.Description != "" {
		return info.Description
	}

	return builder.DefaultDescription
}
