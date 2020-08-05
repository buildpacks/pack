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
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", -1, "Detection Order inspection depth, omission of this flag or values < 0 will display the entire tree")
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
  Buildpack API: {{- if .Info.Lifecycle.API.BuildpackVersion }} {{ .Info.Lifecycle.API.BuildpackVersion }}{{- else }} (none){{- end }}
  Platform API: {{- if .Info.Lifecycle.API.PlatformVersion }} {{ .Info.Lifecycle.API.PlatformVersion }}{{- else }} (none){{- end }}

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
		warnings = append(warnings, fmt.Sprintf("%s does not specify lifecycle version", style.Symbol(imageName)))
	}

	if lcDescriptor.API.BuildpackVersion == nil {
		warnings = append(warnings, fmt.Sprintf("%s does not specify lifecycle buildpack api version", style.Symbol(imageName)))
	}

	if lcDescriptor.API.PlatformVersion == nil {
		warnings = append(warnings, fmt.Sprintf("%s does not specify lifecycle platform api version", style.Symbol(imageName)))
	}

	trustedString := "No"
	if isTrustedBuilder(cfg, imageName) {
		trustedString = "Yes"
	}

	return warnings, tpl.Execute(writer, &struct {
		Info       pack.BuilderInfo
		Buildpacks string
		RunImages  string
		Order      string
		Verbose    bool
		Trusted    string
	}{
		info,
		bps,
		runImgs,
		order,
		verbose,
		trustedString,
	})
}

// TODO: present buildpack order (inc. nested) [https://github.com/buildpacks/pack/issues/253].
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

type stackEntry struct {
	LayerInfo dist.BuildpackRef
	Depth     int
	Last      bool
}

func detectionOrderOutput(order dist.Order, layers dist.BuildpackLayers, maxDepth int) (string, error) {
	buf := strings.Builder{}
	tabWriter := new(tabwriter.Writer).Init(&buf, writerMinWidth, writerTabWidth, defaultTabWidth, writerPadChar, writerFlags)
	invalidMaxDepth := maxDepth == -1

	// the stack for DFS
	buildpackStack := make([]stackEntry, 0)
	prevDepth := -1
	groupCount := 0

	// keep track of buildpacks the current buildpack is nested inside for cycle detection.
	buildpacksDepthStack := []stackEntry{}

	// optimize lookup in buildpackDepthStack.
	buildpackSet := map[pack.BuildpackInfoKey]bool{}

	//initialize stack with top level buildpacks.
	buildpackStack = addOrdering(buildpackStack, order, 0)

	// iterate until stack is empty
	for len(buildpackStack) > 0 {
		stackLen := len(buildpackStack)

		// get current entry an pop last element off the stack
		curEntry := buildpackStack[stackLen-1]
		buildpackStack = buildpackStack[:stackLen-1]

		key := pack.BuildpackInfoKey{
			ID:      curEntry.LayerInfo.ID,
			Version: curEntry.LayerInfo.Version,
		}

		buildpacksDepthStack, buildpackSet = updateCycleChecking(buildpacksDepthStack, buildpackSet, curEntry.Depth)

		_, visited := buildpackSet[key]
		buildpacksDepthStack = append(buildpacksDepthStack, curEntry)
		buildpackSet[key] = true

		curLayerInfo, ok := layers.Get(curEntry.LayerInfo.ID, curEntry.LayerInfo.Version)
		if !ok {
			return "", fmt.Errorf("error: missing buildpack %s@%s from layer metadata", curEntry.LayerInfo.ID, curEntry.LayerInfo.Version)
		}

		// add all nested buildpacks if this buildpack is not the first node of a cycle,
		// or we exceed the maxDepth
		if !visited && (invalidMaxDepth || curEntry.Depth+1 < maxDepth) {
			buildpackStack = addOrdering(buildpackStack, curLayerInfo.Order, curEntry.Depth+1)
		}

		// output operations
		if curEntry.Depth > prevDepth {
			if err := detectionOrderAddGroup(tabWriter, groupCount+1, curEntry.Depth); err != nil {
				return "", fmt.Errorf("unable to add group to output: %s", err)
			}
			groupCount++
		}
		if err := detectionOrderAddBuildpack(tabWriter, curEntry.LayerInfo, curEntry.Depth, visited); err != nil {
			return "", fmt.Errorf("unable to add buildpack to output: %s", err)
		}

		prevDepth = curEntry.Depth
	}

	if err := tabWriter.Flush(); err != nil {
		return "", fmt.Errorf("error flushing tabWriter output: %s", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// pop all buildpacks of greater depth off depth stack,
// we can no longer have a cycle within them.
func updateCycleChecking(buildpacksDepthStack []stackEntry, buildpackSet map[pack.BuildpackInfoKey]bool, newDepth int) ([]stackEntry, map[pack.BuildpackInfoKey]bool) {
	for len(buildpacksDepthStack) > 0 && buildpacksDepthStack[len(buildpacksDepthStack)-1].Depth > newDepth {
		buildpackToRemove := buildpacksDepthStack[len(buildpacksDepthStack)-1]
		buildpacksDepthStack = buildpacksDepthStack[:len(buildpacksDepthStack)-1]
		delete(buildpackSet, pack.BuildpackInfoKey{
			ID:      buildpackToRemove.LayerInfo.ID,
			Version: buildpackToRemove.LayerInfo.Version,
		})
	}
	return buildpacksDepthStack, buildpackSet
}

// add nested buildpack entries from nextOrder onto our stack
func addOrdering(stack []stackEntry, nextOrder dist.Order, nextDepth int) []stackEntry {
	newEntries := []stackEntry{}
	for _, group := range nextOrder {
		for bpIndex, bp := range group.Group {
			newEntries = append(newEntries, stackEntry{
				LayerInfo: bp,
				Depth:     nextDepth,
				Last:      bpIndex == len(group.Group),
			})
		}
	}
	// reverse to do a preorder traversal of buildpack nesting.
	return append(stack, reverseStack(newEntries)...)
}

func reverseStack(stack []stackEntry) []stackEntry {
	left := 0
	right := len(stack) - 1
	for left < right {
		stack[left], stack[right] = stack[right], stack[left]
		left++
		right--
	}
	return stack
}

func detectionOrderAddBuildpack(w io.Writer, buildpack dist.BuildpackRef, depth int, visited bool) error {
	var prefix string
	var optional string
	if buildpack.Optional {
		optional = "(optional)"
	}

	prefix = strings.Repeat("  ", (depth+1)*2)
	visitedStatus := ""
	if visited {
		visitedStatus = "*"
	}

	bpRef := buildpack.ID
	if buildpack.Version != "" {
		bpRef += "@" + buildpack.Version
	}

	_, err := fmt.Fprintf(w, "%s%s%s\t%s\n", prefix, bpRef, visitedStatus, optional)
	return err
}

func detectionOrderAddGroup(w io.Writer, groupCount, depth int) error {
	prefix := strings.Repeat("  ", (depth*2)+1)
	_, err := fmt.Fprintf(w, "%sGroup #%d:\n", prefix, groupCount)
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
