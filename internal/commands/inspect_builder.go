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

const (
	writerMinWidth     = 0
	writerTabWidth     = 0
	buildpacksTabWidth = 8
	defaultTabWidth    = 4
	writerPadChar      = ' '
	writerFlags        = 0
	none               = "(none)"
)

type InspectBuilderFlags struct {
	Depth int
}

func InspectBuilder(logger logging.Logger, cfg config.Config, client PackClient) *cobra.Command {
	var flags InspectBuilderFlags
	cmd := &cobra.Command{
		Use:   "inspect-builder <builder-image-name>",
		Short: "Show information about a builder",
		Args:  cobra.MaximumNArgs(2),
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
			presentRemote, remoteOutput, remoteWarnings, remoteErr := inspectBuilderOutput(client, cfg, imageName, false, verbose, flags.Depth)
			presentLocal, localOutput, localWarnings, localErr := inspectBuilderOutput(client, cfg, imageName, true, verbose, flags.Depth)

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
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", -1, "Max depth to display for Detection Order.\nOmission of this flag or values < 0 will display the entire tree.")
	AddHelpFlag(cmd, "inspect-builder")
	return cmd
}

func inspectBuilderOutput(client PackClient, cfg config.Config, imageName string, local bool, verbose bool, depth int) (present bool, output string, warning []string, err error) {
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
	warnings, err := generateBuilderOutput(&buf, imageName, cfg, *info, verbose, depth)
	if err != nil {
		return true, "", nil, errors.Wrapf(err, "writing output for %s image '%s'", source, imageName)
	}

	return true, buf.String(), warnings, nil
}

func generateBuilderOutput(writer io.Writer, imageName string, cfg config.Config, info pack.BuilderInfo, verbose bool, depth int) (warnings []string, err error) {
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

	order, err := detectionOrderOutput(info.Order, info.BuildpackLayers, depth)
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

	deprecatedBuildpackAPIs := stringifyAPISet(lcDescriptor.APIs.Buildpack.Deprecated)
	supportedBuildpackAPIs := stringifyAPISet(lcDescriptor.APIs.Buildpack.Supported)
	deprecatedPlatformAPIs := stringifyAPISet(lcDescriptor.APIs.Platform.Deprecated)
	supportedPlatformAPIs := stringifyAPISet(lcDescriptor.APIs.Platform.Supported)

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

func buildpacksOutput(bps []dist.BuildpackInfo) (string, error) {
	buf := &bytes.Buffer{}
	tabWriter := new(tabwriter.Writer).Init(buf, writerMinWidth, writerPadChar, buildpacksTabWidth, writerPadChar, writerFlags)
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

	tabWriter := new(tabwriter.Writer).Init(buf, writerMinWidth, writerTabWidth, defaultTabWidth, writerPadChar, writerFlags)

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

// Unable to easily convert format makes this feel like a poor solution...
func detectionOrderOutput(order dist.Order, layers dist.BuildpackLayers, maxDepth int) (string, error) {
	buf := strings.Builder{}
	tabWriter := new(tabwriter.Writer).Init(&buf, writerMinWidth, writerTabWidth, defaultTabWidth, writerPadChar, writerFlags)
	buildpackSet := map[pack.BuildpackInfoKey]bool{}

	if err := orderOutputRecurrence(tabWriter, "", order, layers, buildpackSet, 0, maxDepth); err != nil {
		return "", err
	}
	if err := tabWriter.Flush(); err != nil {
		return "", fmt.Errorf("error flushing tabWriter output: %s", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// Recursively generate output for every buildpack in an order.
func orderOutputRecurrence(w io.Writer, prefix string, order dist.Order, layers dist.BuildpackLayers, buildpackSet map[pack.BuildpackInfoKey]bool, curDepth, maxDepth int) error {
	// exit if maxDepth is exceeded
	if validMaxDepth(maxDepth) && maxDepth <= curDepth {
		return nil
	}

	// otherwise iterate over all nested buildpacks
	for groupIndex, group := range order {
		lastGroup := groupIndex == (len(order) - 1)
		if err := displayGroup(w, prefix, groupIndex+1, lastGroup); err != nil {
			return fmt.Errorf("error when printing group info: %q", err)
		}
		for bpIndex, buildpackEntry := range group.Group {
			lastBuildpack := bpIndex == len(group.Group)-1

			key := pack.BuildpackInfoKey{
				ID:      buildpackEntry.ID,
				Version: buildpackEntry.Version,
			}
			_, visited := buildpackSet[key]
			buildpackSet[key] = true

			curBuildpackLayer, ok := layers.Get(buildpackEntry.ID, buildpackEntry.Version)
			if !ok {
				return fmt.Errorf("error: missing buildpack %s@%s from layer metadata", buildpackEntry.ID, buildpackEntry.Version)
			}

			newBuildpackPrefix := updatePrefix(prefix, lastGroup)
			if err := displayBuildpack(w, newBuildpackPrefix, buildpackEntry, visited, bpIndex == len(group.Group)-1); err != nil {
				return fmt.Errorf("error when printing buildpack info: %q", err)
			}

			newGroupPrefix := updatePrefix(newBuildpackPrefix, lastBuildpack)
			if !visited {
				if err := orderOutputRecurrence(w, newGroupPrefix, curBuildpackLayer.Order, layers, buildpackSet, curDepth+1, maxDepth); err != nil {
					return err
				}
			}

			// remove key from set after recurrence completes, so we only detect cycles.
			delete(buildpackSet, key)
		}
	}
	return nil
}

const (
	branchPrefix     = " ├ "
	lastBranchPrefix = " └ "
	trunkPrefix      = " │ "
)

func updatePrefix(oldPrefix string, last bool) string {
	if last {
		return oldPrefix + "   "
	}
	return oldPrefix + trunkPrefix
}

func validMaxDepth(depth int) bool {
	return depth >= 0
}

func displayGroup(w io.Writer, prefix string, groupCount int, last bool) error {
	treePrefix := branchPrefix
	if last {
		treePrefix = lastBranchPrefix
	}
	_, err := fmt.Fprintf(w, "%s%sGroup #%d:\n", prefix, treePrefix, groupCount)
	return err
}

func displayBuildpack(w io.Writer, prefix string, entry dist.BuildpackRef, visited bool, last bool) error {
	var optional string
	if entry.Optional {
		optional = "(optional)"
	}

	visitedStatus := ""
	if visited {
		visitedStatus = "[cyclic]"
	}

	bpRef := entry.ID
	if entry.Version != "" {
		bpRef += "@" + entry.Version
	}

	treePrefix := branchPrefix
	if last {
		treePrefix = lastBranchPrefix
	}

	_, err := fmt.Fprintf(w, "%s%s%s\t%s%s\n", prefix, treePrefix, bpRef, optional, visitedStatus)
	return err
}

func getLocalMirrors(runImage string, cfg config.Config) []string {
	for _, ri := range cfg.RunImages {
		if ri.Image == runImage {
			return ri.Mirrors
		}
	}
	return nil
}
