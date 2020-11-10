package writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type HumanReadable struct{}

func NewHumanReadable() *HumanReadable {
	return &HumanReadable{}
}

func (h *HumanReadable) Print(
	logger logging.Logger,
	sharedInfo *SharedImageInfo,
	local, remote *pack.ImageInfo,
	localErr, remoteErr error,
) error {
	if local == nil && remote == nil {
		return fmt.Errorf("unable to find image '%s' locally or remotely", sharedInfo.Name)
	}

	localMirrorsFromConfig := getConfiguredMirrors(local, sharedInfo.RunImageMirrors)
	remoteMirrorsFromConfig := getConfiguredMirrors(remote, sharedInfo.RunImageMirrors)

	logger.Infof("Inspecting image: %s\n", style.Symbol(sharedInfo.Name))

	logger.Info("\nREMOTE:\n")
	err := writeImageInfo(logger, remote, remoteMirrorsFromConfig, remoteErr)
	if err != nil {
		return fmt.Errorf("writing remote builder info: %w", err)
	}
	logger.Info("\nLOCAL:\n")
	err = writeImageInfo(logger, local, localMirrorsFromConfig, localErr)
	if err != nil {
		return fmt.Errorf("writing local builder info: %w", err)
	}

	return nil
}

type bom struct {
	Remote interface{} `json:"remote"`
	Local  interface{} `json:"local"`
}

func logBOM(remote *pack.ImageInfo, local *pack.ImageInfo, logger logging.Logger) error {
	var remoteBOM, localBOM interface{}
	if remote != nil {
		remoteBOM = remote.BOM
	}
	if local != nil {
		localBOM = local.BOM
	}
	rawBOM, err := json.Marshal(bom{
		Remote: remoteBOM,
		Local:  localBOM,
	})
	if err != nil {
		return errors.Wrapf(err, "writing bill of materials")
	}
	logger.Infof(string(rawBOM))
	return nil
}

func writeImageInfo(
	logger logging.Logger,
	info *pack.ImageInfo,
	cfgMirrors []string,
	err error,
) error {
	imgTpl := template.Must(template.New("runImages").Parse(runImagesTemplate))
	imgTpl = template.Must(imgTpl.New("buildpacks").Parse(buildpacksTemplate))
	imgTpl = template.Must(imgTpl.New("processes").Parse(processesTemplate))
	imgTpl = template.Must(imgTpl.New("image").Parse(imageTemplate))
	if err != nil {
		logger.Errorf("%s\n", err)
		return nil
	}

	if info == nil {
		logger.Info("(not present)\n")
		return nil
	}
	remoteOutput, err := inspectImageOutput(info, cfgMirrors, imgTpl)
	if err != nil {
		logger.Error(err.Error())
	} else {
		logger.Info(remoteOutput)
	}
	return nil
}

type process struct {
	PType   string
	Shell   string
	Command string
	Args    string
}

func inspectImageOutput(
	info *pack.ImageInfo,
	cfgMirrors []string,
	tpl *template.Template,
) (output string, err error) {
	if info == nil {
		return "(not present)", nil
	}
	var buf bytes.Buffer
	processes := displayProcesses(info.Processes)
	tw := tabwriter.NewWriter(&buf, 0, 0, 8, ' ', 0)
	defer tw.Flush()
	if err := tpl.Execute(tw, &struct {
		Info         *pack.ImageInfo
		LocalMirrors []string
		Processes    []process
	}{
		info,
		cfgMirrors,
		processes,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func displayProcess(p launch.Process, d bool) process {
	shell := ""
	if !p.Direct {
		shell = "bash"
	}

	pType := p.Type
	if d {
		pType = fmt.Sprintf("%s (default)", pType)
	}

	return process{
		PType:   pType,
		Shell:   shell,
		Command: p.Command,
		Args:    strings.Join(p.Args, " "),
	}
}

func displayProcesses(sourceProcesses pack.ProcessDetails) []process {
	processes := []process{}

	if sourceProcesses.DefaultProcess != nil {
		processes = append(processes, displayProcess(*sourceProcesses.DefaultProcess, true))
	}

	for _, p := range sourceProcesses.OtherProcesses {
		processes = append(processes, displayProcess(p, false))
	}

	return processes
}

func getConfiguredMirrors(info *pack.ImageInfo, imageMirrors []config.RunImage) []string {
	var runImage string
	if info != nil {
		runImage = info.Stack.RunImage.Image
	}

	for _, ri := range imageMirrors {
		if ri.Image == runImage {
			return ri.Mirrors
		}
	}
	return nil
}

var runImagesTemplate = `
Run Images:
{{- range $_, $m := .LocalMirrors }}
  {{$m}}	(user-configured)
{{- end }}
{{- if .Info.Stack.RunImage.Image }}
  {{ .Info.Stack.RunImage.Image }}
{{- else }}
  (none)
{{- end }}
{{- range $_, $m := .Info.Stack.RunImage.Mirrors }}
  {{$m}}
{{- end }}`

var buildpacksTemplate = `
Buildpacks:
{{- if .Info.Buildpacks }}
  ID	VERSION
{{- range $_, $b := .Info.Buildpacks }}
  {{ $b.ID }}	{{ $b.Version }}
{{- end }}
{{- else }}
  (buildpacks metadata not present)
{{- end }}`

var processesTemplate = `
{{- if .Processes }}

Processes:
  TYPE	SHELL	COMMAND	ARGS
{{- range $_, $p := .Processes }}
  {{ $p.PType }}	{{ $p.Shell }}	{{ $p.Command }}	{{ $p.Args }}
{{- end }}
{{- end }}`

var imageTemplate = `
Stack: {{ .Info.StackID }}

Base Image:
{{- if .Info.Base.Reference}}
  Reference: {{ .Info.Base.Reference }}
{{- end}}
  Top Layer: {{ .Info.Base.TopLayer }}
{{ template "runImages" . }}
{{ template "buildpacks" . }}{{ template "processes" . }}

`
