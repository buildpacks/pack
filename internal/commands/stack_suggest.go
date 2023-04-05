package commands

import (
	"bytes"
	"html/template"
	"sort"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/pkg/logging"
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
		ID:          "Deprecation Notice",
		Description: "Stacks will continue to be supported through at least 2024 but are deprecated in favor of using BuildImages and RunImages directly. Please see our docs for more details- https://buildpacks.io/docs/concepts/components/stack",
		Maintainer:  "CNB",
	},
	{
		ID:          "heroku-20",
		Description: "The official Heroku stack based on Ubuntu 20.04",
		Maintainer:  "Heroku",
		BuildImage:  "heroku/heroku:20-cnb-build",
		RunImage:    "heroku/heroku:20-cnb",
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

func stackSuggest(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suggest",
		Args:    cobra.NoArgs,
		Short:   "(deprecated) List the recommended stacks",
		Example: "pack stack suggest",
		RunE: logError(logger, func(*cobra.Command, []string) error {
			Suggest(logger)
			return nil
		}),
	}

	return cmd
}

func Suggest(log logging.Logger) {
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
