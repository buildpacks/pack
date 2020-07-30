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
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", 0, "Detection Order inspection depth")
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

func detectionOrderOutput(order dist.Order, layers dist.BuildpackLayers, maxDepth int) (string, error) {
	buf := strings.Builder{}
	tabWriter := new(tabwriter.Writer).Init(&buf, 0, 0, 4, ' ', 0)
	orderOutputWriter := NewOrderOutputWriter(tabWriter, order, layers)
	err := orderOutputWriter.GenerateOutput(maxDepth)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// generate mock
type TabWriter interface {
	Flush() error
	Write(buf []byte) (n int, err error)
}

type OrderOutputWriter struct {
	// turn this into an interface
	twriter       TabWriter
	builderLayers dist.BuildpackLayers
	builderOrder  dist.Order
	groupCount    int
	visitedMap    map[string]bool
}

func NewOrderOutputWriter(twriter TabWriter, builderOrder dist.Order, builderLayers dist.BuildpackLayers) OrderOutputWriter {
	return OrderOutputWriter{
		twriter:       twriter,
		builderOrder:  builderOrder,
		builderLayers: builderLayers,
		groupCount:    0,
		visitedMap:    map[string]bool{},
	}
}

func (o *OrderOutputWriter) Reset() {
	o.groupCount = 0
	o.visitedMap = map[string]bool{}
}

func (o *OrderOutputWriter) GenerateOutput(maxDepth int) error {
	o.Reset()

	defer o.Reset()
	//all output goes into buffer
	for _, group := range o.builderOrder {
		o.groupCount += 1
		if err := o.genGroupOutput(1); err != nil {
			return err
		}
		for _, bp := range group.Group {
			if err := o.genNestedOutput(bp, 1, maxDepth); err != nil {
				return err
			}
		}
	}
	return o.twriter.Flush()
}

func (o *OrderOutputWriter) genNestedOutput(start dist.BuildpackRef, depth, maxDepth int) error {
	// check for max depth

	// check for cycle
	key := fmt.Sprintf("%s@%s", start.ID, start.Version)
	if _, ok := o.visitedMap[key]; ok {
		return fmt.Errorf("circular dependency detected in group ordering")
	}
	o.visitedMap[key] = true

	buildpackEntries, ok := o.builderLayers[start.ID]
	if !ok {
		panic(fmt.Errorf("buildpack %s not found in layers map", start.ID))
	}
	// get the entry key (if non exists and map length is size 1, then use the present key)
	entryVersion := start.Version

	// TODO make this more readable, pretty bad right now.
	if entryVersion == "" && len(buildpackEntries) == 1 {
		for key, _ := range buildpackEntries {
			entryVersion = key
		}
	}
	buildpackEntry, ok := buildpackEntries[entryVersion]
	if !ok {
		panic(fmt.Errorf("buildpack version %s not found in layers map at %#v", entryVersion, buildpackEntries))
	}

	o.genBuildpackOutput(start, depth*2)

	if maxDepth != 0 && depth >= maxDepth {
		return nil
	}

	if len(buildpackEntry.Order) == 0 {
		return nil
	}
	for _, group := range buildpackEntry.Order {
		o.groupCount += 1
		if err := o.genGroupOutput(depth*2 + 1); err != nil {
			return err
		}
		for _, bp := range group.Group {
			if err := o.genNestedOutput(bp, depth+1, maxDepth); err != nil {
				return err
			}
		}
	}

	return nil
}

func (o *OrderOutputWriter) genBuildpackOutput(buildpack dist.BuildpackRef, indentLevel int) error {
	var prefix string
	var optional string
	if buildpack.Optional {
		optional = "(optional)"
	}

	prefix = strings.Repeat("  ", indentLevel)

	bpRef := buildpack.ID
	if buildpack.Version != "" {
		bpRef += "@" + buildpack.Version
	}

	_, err := fmt.Fprintf(o.twriter, "%s%s\t%s\n", prefix, bpRef, optional)
	return err
}

func (o *OrderOutputWriter) genGroupOutput(indentLevel int) error {
	prefix := strings.Repeat("  ", indentLevel)
	_, err := fmt.Fprintf(o.twriter, "%sGroup #%d:\n", prefix, o.groupCount)
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
