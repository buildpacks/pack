package commands

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/image"

	"github.com/buildpacks/pack/internal/buildpack"

	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/buildpackage"

	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

const inspectBuildpackTemplate = `
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

Buildpacks:
{{ .Buildpacks }}

Detection Order:
{{- if ne .Order "" }}
{{ .Order }}
{{- else }}
  (none)
{{ end }}
`

type InspectBuildpackFlags struct {
	Depth    int
	Registry string
	Verbose  bool
}

func InspectBuildpack(logger logging.Logger, cfg *config.Config, client PackClient) *cobra.Command {
	var flags InspectBuildpackFlags
	cmd := &cobra.Command{
		Use:     "inspect-buildpack <image-name>",
		Args:    cobra.RangeArgs(1, 4),
		Short:   "Show information about a buildpack",
		Example: "pack inspect-buildpack cnbs/sample-package:hello-universe",
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			buildpackName := args[0]
			registry := flags.Registry
			if registry == "" {
				//nolint:staticcheck
				registry = cfg.DefaultRegistry
			}

			logger.Infof("Inspecting buildpack: %s\n", style.Symbol(buildpackName))

			inspectedBuildpacksOutput, err := inspectAllBuildpacks(
				client,
				flags,
				pack.InspectBuildpackOptions{
					BuildpackName: buildpackName,
					Daemon:        true,
					Registry:      registry,
				},
				pack.InspectBuildpackOptions{
					BuildpackName: buildpackName,
					Daemon:        false,
					Registry:      registry,
				})
			if err != nil {
				return fmt.Errorf("error writing buildpack output: %q", err)
			}

			logger.Info(inspectedBuildpacksOutput)
			return nil
		}),
	}
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", -1, "Max depth to display for Detection Order.\nOmission of this flag or values < 0 will display the entire tree.")
	cmd.Flags().StringVarP(&flags.Registry, "registry", "r", "", "buildpack registry that may be searched")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "show more output")
	AddHelpFlag(cmd, "inspect-buildpack")
	return cmd
}

func inspectAllBuildpacks(client PackClient, flags InspectBuildpackFlags, options ...pack.InspectBuildpackOptions) (string, error) {
	buf := bytes.NewBuffer(nil)
	skipCount := 0
	for _, option := range options {
		nextResult, err := client.InspectBuildpack(option)
		if err != nil {
			if errors.Is(err, image.ErrNotFound) {
				skipCount++
				continue
			}
			return "", err
		}

		prefix := determinePrefix(option.BuildpackName, nextResult.Location, option.Daemon)

		output, err := inspectBuildpackOutput(nextResult, prefix, flags)
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
	if skipCount == len(options) {
		return "", errors.New("no buildpacks found")
	}
	return buf.String(), nil
}

func inspectBuildpackOutput(info *pack.BuildpackInfo, prefix string, flags InspectBuildpackFlags) (output []byte, err error) {
	tpl := template.Must(template.New("inspect-buildpack").Parse(inspectBuildpackTemplate))
	bpOutput, err := buildpacksOutput(info.Buildpacks)
	if err != nil {
		return []byte{}, fmt.Errorf("error writing buildpack output: %q", err)
	}
	orderOutput, err := detectionOrderOutput(info.Order, info.BuildpackLayers, flags.Depth)
	if err != nil {
		return []byte{}, fmt.Errorf("error writing detection order output: %q", err)
	}
	buf := bytes.NewBuffer(nil)

	err = tpl.Execute(buf, &struct {
		Location   string
		Metadata   buildpackage.Metadata
		ListMixins bool
		Buildpacks string
		Order      string
	}{
		Location:   prefix,
		Metadata:   info.BuildpackMetadata,
		ListMixins: flags.Verbose,
		Buildpacks: bpOutput,
		Order:      orderOutput,
	})

	if err != nil {
		return []byte{}, fmt.Errorf("error templating buildpack output template: %q", err)
	}
	return buf.Bytes(), nil
}

func determinePrefix(name string, locator buildpack.LocatorType, daemon bool) string {
	switch locator {
	case buildpack.RegistryLocator:
		return "REGISTRY IMAGE"
	case buildpack.PackageLocator:
		if daemon {
			return "LOCAL IMAGE"
		}
		return "REMOTE IMAGE"
	case buildpack.URILocator:
		if strings.HasPrefix(name, "http") {
			return "REMOTE ARCHIVE"
		}
		return "LOCAL ARCHIVE"
	}
	return "UNKNOWN SOURCE"
}
