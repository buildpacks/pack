package commands

import (
	"bytes"
	"html/template"
	"sort"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/logging"
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
		Description: "A minimal Paketo stack based on Ubuntu 18.04",
		Maintainer:  "Paketo Project",
		BuildImage:  "paketobuildpacks/build:base-cnb",
		RunImage:    "paketobuildpacks/run:base-cnb",
	},
	{
		ID:          "io.buildpacks.stacks.bionic",
		Description: "A large Paketo stack based on Ubuntu 18.04",
		Maintainer:  "Paketo Project",
		BuildImage:  "paketobuildpacks/build:full-cnb",
		RunImage:    "paketobuildpacks/run:full-cnb",
	},
	{
		ID:          "io.paketo.stacks.tiny",
		Description: "A tiny Paketo stack based on Ubuntu 18.04, similar to distroless",
		Maintainer:  "Paketo Project",
		BuildImage:  "paketobuildpacks/build:tiny-cnb",
		RunImage:    "paketobuildpacks/run:tiny-cnb",
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
	tmpl := template.Must(template.New("").Parse(`Stacks maintained by the community:
{{- range . }}

    Stack ID: {{ .ID }}
    Description: {{ .Description }}
    Maintainer: {{ .Maintainer }}
    Build Image: {{ .BuildImage }}
    Run Image: {{ .RunImage }}
{{- end }}
`))

	buf := &bytes.Buffer{}
	tmpl.Execute(buf, suggestedStacks)
	log.Info(buf.String())
}
