package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack/internal/build"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/pkg/logging"
)

// quotedValueRe matches any double-quoted string in a TOML line.
// Pre-compiled once at package initialisation to avoid repeated allocation
// inside the hot sanitize() path.
var quotedValueRe = regexp.MustCompile(`"(.*?)"`)

func Report(logger logging.Logger, version, cfgPath string) *cobra.Command {
	var explicit bool

	cmd := &cobra.Command{
		Use:     "report",
		Args:    cobra.NoArgs,
		Short:   "Display useful information for reporting an issue",
		Example: "pack report",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			var buf bytes.Buffer
			err := generateOutput(&buf, version, cfgPath, explicit)
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

func generateOutput(writer io.Writer, version, cfgPath string, explicit bool) error {
	tpl := template.Must(template.New("").Parse(`Pack:
  Version:  {{ .Version }}
  OS/Arch:  {{ .OS }}/{{ .Arch }}

Default Lifecycle Version:  {{ .DefaultLifecycleVersion }}

Supported Platform APIs:  {{ .SupportedPlatformAPIs }}

Config:
{{ .Config -}}`))

	configData := ""
	if data, err := os.ReadFile(filepath.Clean(cfgPath)); err != nil {
		configData = fmt.Sprintf("(no config file found at %s)", cfgPath)
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

	platformAPIs := strings.Join(build.SupportedPlatformAPIVersions.AsStrings(), ", ")

	return tpl.Execute(writer, map[string]string{
		"Version":                 version,
		"OS":                      runtime.GOOS,
		"Arch":                    runtime.GOARCH,
		"DefaultLifecycleVersion": builder.DefaultLifecycleVersion,
		"SupportedPlatformAPIs":   platformAPIs,
		"Config":                  configData,
	})
}

// sanitize redacts the quoted values of sensitive TOML fields in a config
// line so that they are not leaked by `pack report`.
//
// The match is intentionally exact: a TOML key is considered sensitive only
// when the trimmed line begins with `<field-name>` followed immediately by
// optional whitespace and `=`. This prevents false-positive redaction of
// keys that merely share a prefix with a sensitive field (e.g. an
// "image-mirror" key would previously be redacted because it starts with
// the sensitive field "image").
func sanitize(line string) string {
	sensitiveFields := []string{
		"default-builder-image",
		"image",
		"mirrors",
		"name",
		"url",
	}
	for _, field := range sensitiveFields {
		// Build a pattern that matches the exact TOML key:
		//   ^\s*<field>\s*=
		// This ensures "image" does not match "image-mirror" etc.
		keyRe := regexp.MustCompile(`(?i)^\s*` + regexp.QuoteMeta(field) + `\s*=`)
		if keyRe.MatchString(line) {
			return quotedValueRe.ReplaceAllString(line, `"[REDACTED]"`)
		}
	}

	return line
}
