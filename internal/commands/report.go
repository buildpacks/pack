package commands

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"runtime"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/buildpack/pack/cmd"
	"github.com/buildpack/pack/internal/builder"
	"github.com/buildpack/pack/internal/config"
	"github.com/buildpack/pack/logging"
)

func Report(logger logging.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Args:  cobra.NoArgs,
		Short: "Display useful information for reporting an issue",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			var buf bytes.Buffer
			err := generateOutput(&buf)
			if err != nil {
				return err
			}

			logger.Info(buf.String())

			return nil
		}),
	}
	AddHelpFlag(cmd, "report")
	return cmd
}

func generateOutput(writer io.Writer) error {
	tpl := template.Must(template.New("").Parse(`Pack:
  Version:  {{ .Version }}
  OS/Arch:  {{ .OS }}/{{ .Arch }}

Default Lifecycle Version:  {{ .DefaultLifecycleVersion }}

Config:
{{ .Config -}}`))

	configData := ""
	if path, err := config.DefaultConfigPath(); err != nil {
		configData = fmt.Sprintf("(error: %s)", err.Error())
	} else if data, err := ioutil.ReadFile(path); err != nil {
		configData = fmt.Sprintf("(error: %s)", err.Error())
	} else {
		var padded strings.Builder
		for _, line := range strings.Split(string(data), "\n") {
			_, _ = fmt.Fprintf(&padded, "  %s\n", line)
		}
		configData = strings.TrimRight(padded.String(), " \n")
	}

	return tpl.Execute(writer, map[string]string{
		"Version":                 cmd.Version,
		"OS":                      runtime.GOOS,
		"Arch":                    runtime.GOARCH,
		"DefaultLifecycleVersion": builder.DefaultLifecycleVersion,
		"Config":                  configData,
	})
}
