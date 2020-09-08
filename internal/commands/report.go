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

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/logging"
)

func Report(logger logging.Logger, version string) *cobra.Command {
	var explicit bool

	cmd := &cobra.Command{
		Use:   "report",
		Args:  cobra.NoArgs,
		Short: "Display useful information for reporting an issue",
		RunE: LogError(logger, func(cmd *cobra.Command, args []string) error {
			var buf bytes.Buffer
			err := generateOutput(&buf, version, explicit)
			if err != nil {
				return err
			}

			logger.Info(buf.String())

			return nil
		}),
	}

	cmd.Flags().BoolVarP(&explicit, "explicit", "e", false, "Print config without redacting information")
	AddHelpFlag(cmd, "report")
	return cmd
}

func generateOutput(writer io.Writer, version string, explicit bool) error {
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
		configData = fmt.Sprintf("(no config file found at %s)", path)
	} else {
		var padded strings.Builder

		for _, line := range strings.Split(string(data), "\n") {
			if !explicit {
				line = sanitize(line)
			}
			_, _ = fmt.Fprintf(&padded, "  %s\n", line)
		}
		configData = strings.TrimRight(padded.String(), " \n")
	}

	return tpl.Execute(writer, map[string]string{
		"Version":                 version,
		"OS":                      runtime.GOOS,
		"Arch":                    runtime.GOARCH,
		"DefaultLifecycleVersion": builder.DefaultLifecycleVersion,
		"Config":                  configData,
	})
}

func sanitize(line string) string {
	if strings.HasPrefix(line, "default-builder-image") {
		return `default-builder-image = "[REDACTED]"`
	}

	return line
}
