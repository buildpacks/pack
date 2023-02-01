package commands

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/client"
)

const inspectExtensionTemplate = `
{{ .Location -}}:

Stacks:
{{- range $stackIndex, $stack := .Metadata.Stacks }}
  ID: {{ $stack.ID }}
    Mixins:
  {{- if $.ListMixins }}
    {{- if eq (len $stack.Mixins) 0 }}
      (none)
    {{- else }}
      {{- range $mixinIndex, $mixin := $stack.Mixins }}
      {{ $mixin }}
      {{- end }}
    {{- end }}
  {{- else }}
      (omitted)
  {{- end }}
{{- end }}

Extensions:
{{ .Extensions }}

Detection Order:
{{- if ne .Order "" }}
{{ .Order }}
{{- else }}
  (none)
{{ end }}
`

func inspectAllExtensions(client PackClient, flags ExtensionInspectFlags, options ...client.InspectExtensionOptions) (string, error) {
	buf := bytes.NewBuffer(nil)
	errArray := []error{}
	for _, option := range options {
		nextResult, err := client.InspectExtension(option)
		if err != nil {
			errArray = append(errArray, err)
			continue
		}

		prefix := determinePrefix(option.ExtensionName, nextResult.Location, option.Daemon)

		output, err := inspectExtensionOutput(nextResult, prefix, flags)
		if err != nil {
			return "", err
		}

		if _, err := buf.Write(output); err != nil {
			return "", err
		}

		if nextResult.Location != buildpack.PackageLocator {
			return buf.String(), nil
		}
	}
	if len(errArray) == len(options) {
		return "", joinErrors(errArray)
	}
	return buf.String(), nil
}

func inspectExtensionOutput(info *client.ExtensionInfo, prefix string, flags ExtensionInspectFlags) (output []byte, err error) {
	tpl := template.Must(template.New("inspect-extension").Parse(inspectExtensionTemplate))
	exOutput, err := buildpacksOutput(info.Extensions)
	if err != nil {
		return []byte{}, fmt.Errorf("error writing buildpack output: %q", err)
	}
	orderOutput, err := detectionOrderOutput(info.Order, info.ExtensionLayers, flags.Depth)
	if err != nil {
		return []byte{}, fmt.Errorf("error writing detection order output: %q", err)
	}
	buf := bytes.NewBuffer(nil)

	err = tpl.Execute(buf, &struct {
		Location   string
		Metadata   buildpack.Metadata
		ListMixins bool
		Extensions string
		Order      string
	}{
		Location:   prefix,
		Metadata:   info.ExtensionMetadata,
		ListMixins: flags.Verbose,
		Extensions: exOutput,
		Order:      orderOutput,
	})

	if err != nil {
		return []byte{}, fmt.Errorf("error templating buildpack output template: %q", err)
	}
	return buf.Bytes(), nil
}
