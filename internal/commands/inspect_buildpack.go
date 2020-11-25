package commands

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/buildpacks/pack/internal/dist"

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

const (
	writerMinWidth     = 0
	writerTabWidth     = 0
	buildpacksTabWidth = 8
	defaultTabWidth    = 4
	writerPadChar      = ' '
	writerFlags        = 0
)

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
		RunE: LogError(logger, func(cmd *cobra.Command, args []string) error {
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
