package commands

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	"github.com/buildpack/pack/style"
)

func InspectBuilder(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if cfg.DefaultBuilder == "" && len(args) == 0 {
				suggestSettingBuilder(logger, client)
				return MakeSoftError()
			}

			imageName := cfg.DefaultBuilder
			if len(args) >= 1 {
				imageName = args[0]
			}

			if imageName == cfg.DefaultBuilder {
				logger.Infof("Inspecting default builder: %s\n", style.Symbol(imageName))
			} else {
				logger.Infof("Inspecting builder: %s\n", style.Symbol(imageName))
			}

			remoteOutput, warnings, err := inspectBuilderOutput(client, cfg, imageName, false)
			if err != nil {
				logger.Error(err.Error())
			} else {
				logger.Infof("REMOTE:\n%s\n", remoteOutput)
				for _, w := range warnings {
					logger.Warn(w)
				}
			}

			localOutput, warnings, err := inspectBuilderOutput(client, cfg, imageName, true)
			if err != nil {
				logger.Error(err.Error())
			} else {
				logger.Infof("\nLOCAL:\n%s\n", localOutput)
				for _, w := range warnings {
					logger.Warn(w)
				}
			}

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(client PackClient, cfg config.Config, imageName string, local bool) (output string, warning []string, err error) {
	source := "remote"
	if local {
		source = "local"
	}

	info, err := client.InspectBuilder(imageName, local)
	if err != nil {
		return "", nil, errors.Wrapf(err, "inspecting %s image '%s'", source, imageName)
	}

	if info == nil {
		return "(not present)", nil, nil
	}

	var buf bytes.Buffer
	warnings, err := generateOutput(&buf, imageName, cfg, *info)
	if err != nil {
		return "", nil, errors.Wrapf(err, "writing output for %s image '%s'", source, imageName)
	}

	return buf.String(), warnings, nil
}

func generateOutput(writer io.Writer, imageName string, cfg config.Config, info pack.BuilderInfo) (warnings []string, err error) {
	tpl := template.Must(template.New("").Parse(`
{{ if ne .Info.Description "" -}}
Description: {{ .Info.Description }}

{{ end -}}

{{- if ne .Info.CreatedBy.Name "" -}}
Created By:
  Name: {{ .Info.CreatedBy.Name }}
  Version: {{ .Info.CreatedBy.Version }}

{{ end -}}

Stack: {{ .Info.Stack }}

Lifecycle:
  Version: {{ .Info.Lifecycle.Info.Version }}
  Buildpack API: {{ .Info.Lifecycle.API.BuildpackVersion }}
  Platform API: {{ .Info.Lifecycle.API.PlatformVersion }}

Run Images:
{{- if ne .RunImages "" }}
{{ .RunImages }}
{{- else }}
  (none) 
{{- end }}

Buildpacks:
{{- if .Info.Buildpacks }}
{{ .Buildpacks }}
{{- else }}
  (none) 
{{- end }}

Detection Order:
{{- if ne .Order "" }}
{{ .Order }}
{{- else }}
  (none)
{{ end }}`,
	))

	bps, err := buildpacksOutput(info.Buildpacks)
	if err != nil {
		return nil, err
	}

	if len(info.Buildpacks) == 0 {
		warnings = append(warnings, fmt.Sprintf("%s has no buildpacks", style.Symbol(imageName)))
		warnings = append(warnings, "Users must supply buildpacks from the host machine")
	}

	order, err := detectionOrderOutput(info.Groups)
	if err != nil {
		return nil, err
	}

	if len(info.Groups) == 0 {
		warnings = append(warnings, fmt.Sprintf("%s does not specify detection order", style.Symbol(imageName)))
		warnings = append(warnings, "Users must build with explicitly specified buildpacks")
	}

	runImgs, err := runImagesOutput(info.RunImage, info.RunImageMirrors, cfg)
	if err != nil {
		return nil, err
	}

	if info.RunImage == "" {
		warnings = append(warnings, fmt.Sprintf("%s does not specify a run image", style.Symbol(imageName)))
		warnings = append(warnings, "Users must build with an explicitly specified run image")
	}

	lcDescriptor := &info.Lifecycle
	if lcDescriptor.Info.Version == nil {
		lcDescriptor.Info.Version = builder.VersionMustParse(builder.AssumedLifecycleVersion)
	}

	if lcDescriptor.API.BuildpackVersion == nil {
		lcDescriptor.API.BuildpackVersion = api.MustParse(builder.AssumedBuildpackAPIVersion)
	}

	if lcDescriptor.API.PlatformVersion == nil {
		lcDescriptor.API.PlatformVersion = api.MustParse(builder.AssumedPlatformAPIVersion)
	}

	return warnings, tpl.Execute(writer, &struct {
		Info       pack.BuilderInfo
		Buildpacks string
		RunImages  string
		Order      string
	}{
		info,
		bps,
		runImgs,
		order,
	})
}

// TODO: present buildpack order (inc. nested) [https://github.com/buildpack/pack/issues/253].
func buildpacksOutput(bps []builder.BuildpackMetadata) (string, error) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 8, ' ', 0)
	if _, err := fmt.Fprint(tabWriter, "  ID\tVERSION\n"); err != nil {
		return "", err
	}

	for _, bp := range bps {
		if _, err := fmt.Fprint(tabWriter, fmt.Sprintf("  %s\t%s\n", bp.ID, bp.Version)); err != nil {
			return "", err
		}
	}

	if err := tabWriter.Flush(); err != nil {
		return "", err
	}

	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func runImagesOutput(runImage string, mirrors []string, cfg config.Config) (string, error) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 4, ' ', 0)

	for _, r := range getLocalMirrors(runImage, cfg) {
		if _, err := fmt.Fprintf(tabWriter, "  %s\t(user-configured)\n", r); err != nil {
			return "", err
		}
	}

	if runImage != "" {
		if _, err := fmt.Fprintf(tabWriter, "  %s\n", runImage); err != nil {
			return "", err
		}
	}

	for _, r := range mirrors {
		if _, err := fmt.Fprintf(tabWriter, "  %s\n", r); err != nil {
			return "", err
		}
	}

	if err := tabWriter.Flush(); err != nil {
		return "", err
	}

	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func detectionOrderOutput(order builder.Order) (string, error) {
	buf := strings.Builder{}
	for i, group := range order {
		buf.WriteString(fmt.Sprintf("  Group #%d:\n", i+1))

		tabWriter := new(tabwriter.Writer).Init(&buf, 0, 0, 4, ' ', 0)
		for _, bp := range group.Group {
			var optional string
			if bp.Optional {
				optional = "(optional)"
			}

			bpRef := bp.ID
			if bp.Version != "" {
				bpRef += "@" + bp.Version
			}

			if _, err := fmt.Fprintf(tabWriter, "    %s\t%s\n", bpRef, optional); err != nil {
				return "", err
			}
		}
		if err := tabWriter.Flush(); err != nil {
			return "", err
		}
	}

	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func getLocalMirrors(runImage string, cfg config.Config) []string {
	for _, ri := range cfg.RunImages {
		if ri.Image == runImage {
			return ri.Mirrors
		}
	}
	return nil
}
