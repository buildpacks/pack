package commands

import (
	"html/template"
	"sort"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack/logging"
)

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

func SuggestStacks(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest-stacks",
		Short: "Display list of recommended stacks",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			suggestStacks(logger)
		},
	}

	AddHelpFlag(cmd, "suggest-stacks")
	return cmd
}

func suggestStacks(log logging.Logger) {
	sort.Slice(suggestedStacks, func(i, j int) bool { return suggestedStacks[i].ID < suggestedStacks[j].ID })

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
