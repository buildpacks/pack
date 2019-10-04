package commands

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

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

func SuggestBuilders(logger logging.Logger, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-builders",
		Short: "Display list of recommended builders",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
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
