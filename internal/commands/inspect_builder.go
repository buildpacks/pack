package commands

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

const none = "(none)"

func InspectBuilder(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.MaximumNArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			if cfg.DefaultBuilder == "" && len(args) == 0 {
				suggestSettingBuilder(logger, client)
				return pack.NewSoftError()
			}

			imageName := cfg.DefaultBuilder
			if len(args) >= 1 {
				imageName = args[0]
			}

			verbose := logger.IsVerbose()
			presentRemote, remoteOutput, remoteWarnings, remoteErr := inspectBuilderOutput(client, cfg, imageName, false, verbose)
			presentLocal, localOutput, localWarnings, localErr := inspectBuilderOutput(client, cfg, imageName, true, verbose)

			if !presentRemote && !presentLocal {
				return errors.New(fmt.Sprintf("Unable to find builder '%s' locally or remotely.\n", imageName))
			}

			if imageName == cfg.DefaultBuilder {
				logger.Infof("Inspecting default builder: %s\n", style.Symbol(imageName))
			} else {
				logger.Infof("Inspecting builder: %s\n", style.Symbol(imageName))
			}

			if remoteErr != nil {
				logger.Error(remoteErr.Error())
			} else {
				logger.Infof("\nREMOTE:\n%s\n", remoteOutput)
				for _, w := range remoteWarnings {
					logger.Warn(w)
				}
			}

			if localErr != nil {
				logger.Error(localErr.Error())
			} else {
				logger.Infof("\nLOCAL:\n%s\n", localOutput)
				for _, w := range localWarnings {
					logger.Warn(w)
				}
			}

			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(client PackClient, cfg config.Config, imageName string, local bool, verbose bool) (present bool, output string, warning []string, err error) {
	source := "remote"
	if local {
		source = "local"
	}

	info, err := client.InspectBuilder(imageName, local)
	if err != nil {
		return true, "", nil, errors.Wrapf(err, "inspecting %s image '%s'", source, imageName)
	}

	if info == nil {
		return false, "(not present)", nil, nil
	}

	var buf bytes.Buffer
	warnings, err := generateBuilderOutput(&buf, imageName, cfg, *info, verbose)
	if err != nil {
		return true, "", nil, errors.Wrapf(err, "writing output for %s image '%s'", source, imageName)
	}

	return true, buf.String(), warnings, nil
}

func generateBuilderOutput(writer io.Writer, imageName string, cfg config.Config, info pack.BuilderInfo, verbose bool) (warnings []string, err error) {
	tpl := template.Must(template.New("").Parse(`
{{ if ne .Info.Description "" -}}
Description: {{ .Info.Description }}

{{ end -}}

{{- if ne .Info.CreatedBy.Name "" -}}
Created By:
  Name: {{ .Info.CreatedBy.Name }}
  Version: {{ .Info.CreatedBy.Version }}

{{ end -}}

Trusted: {{.Trusted}}

Stack:
  ID: {{ .Info.Stack }}
{{- if .Verbose}}
{{- if ne (len .Info.Mixins) 0 }}
  Mixins:
{{- end }}
{{- range $index, $mixin := .Info.Mixins }}
    {{ $mixin }}
{{- end }}
{{- end }}

Lifecycle:
  Version: {{- if .Info.Lifecycle.Info.Version }} {{ .Info.Lifecycle.Info.Version }}{{- else }} (none){{- end }}
  Buildpack APIs:
    Deprecated: {{ .DeprecatedBuildpackAPIs }}
    Supported: {{ .SupportedBuildpackAPIs }}
  Platform APIs:
    Deprecated: {{ .DeprecatedPlatformAPIs }}
    Supported: {{ .SupportedPlatformAPIs }}

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

	order, err := detectionOrderOutput(info.Order)
	if err != nil {
		return nil, err
	}

	if len(info.Order) == 0 {
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
		warnings = append(warnings, fmt.Sprintf("%s does not specify a Lifecycle version", style.Symbol(imageName)))
	}

	deprecatedBuildpackAPIs := none
	supportedBuildpackAPIs := none
	deprecatedPlatformAPIs := none
	supportedPlatformAPIs := none
	if lcDescriptor.APIs != nil {
		deprecatedBuildpackAPIs = stringifyAPISet(lcDescriptor.APIs.Buildpack.Deprecated)
		supportedBuildpackAPIs = stringifyAPISet(lcDescriptor.APIs.Buildpack.Supported)
		deprecatedPlatformAPIs = stringifyAPISet(lcDescriptor.APIs.Platform.Deprecated)
		supportedPlatformAPIs = stringifyAPISet(lcDescriptor.APIs.Platform.Supported)
	}
	if supportedBuildpackAPIs == none {
		warnings = append(warnings, fmt.Sprintf("%s does not specify supported Lifecycle Buildpack APIs", style.Symbol(imageName)))
	}
	if supportedPlatformAPIs == none {
		warnings = append(warnings, fmt.Sprintf("%s does not specify supported Lifecycle Platform APIs", style.Symbol(imageName)))
	}

	trustedString := "No"
	if isTrustedBuilder(cfg, imageName) {
		trustedString = "Yes"
	}

	return warnings, tpl.Execute(writer, &struct {
		Info                    pack.BuilderInfo
		Buildpacks              string
		RunImages               string
		Order                   string
		Verbose                 bool
		Trusted                 string
		DeprecatedBuildpackAPIs string
		SupportedBuildpackAPIs  string
		DeprecatedPlatformAPIs  string
		SupportedPlatformAPIs   string
	}{
		info,
		bps,
		runImgs,
		order,
		verbose,
		trustedString,
		deprecatedBuildpackAPIs,
		supportedBuildpackAPIs,
		deprecatedPlatformAPIs,
		supportedPlatformAPIs,
	})
}

func stringifyAPISet(versions builder.APISet) string {
	if len(versions) == 0 {
		return none
	}

	return strings.Join(versions.AsStrings(), ", ")
}

// TODO: present buildpack order (inc. nested) [https://github.com/buildpacks/pack/issues/253].
func buildpacksOutput(bps []dist.BuildpackInfo) (string, error) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, 0, 0, 8, ' ', 0)
	if _, err := fmt.Fprint(tabWriter, "  ID\tVERSION\tHOMEPAGE\n"); err != nil {
		return "", err
	}

	for _, bp := range bps {
		if _, err := fmt.Fprintf(tabWriter, "  %s\t%s\t%s\n", bp.ID, bp.Version, bp.Homepage); err != nil {
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

func detectionOrderOutput(order dist.Order) (string, error) {
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
