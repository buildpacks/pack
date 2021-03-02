package writer

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/buildpacks/pack/internal/inspectimage"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/logging"
)

type HumanReadable struct{}

func NewHumanReadable() *HumanReadable {
	return &HumanReadable{}
}

func (h *HumanReadable) Print(
	logger logging.Logger,
	generalInfo inspectimage.GeneralInfo,
	local, remote *pack.ImageInfo,
	localErr, remoteErr error,
) error {
	if local == nil && remote == nil {
		return fmt.Errorf("unable to find image '%s' locally or remotely", generalInfo.Name)
	}

	localDisplay := inspectimage.NewInfoDisplay(local, generalInfo)
	remoteDisplay := inspectimage.NewInfoDisplay(remote, generalInfo)

	logger.Infof("Inspecting image: %s\n", style.Symbol(generalInfo.Name))

	logger.Info("\nREMOTE:\n")
	err := writeImageInfo(logger, remoteDisplay, remoteErr)
	if err != nil {
		return fmt.Errorf("writing remote builder info: %w", err)
	}
	logger.Info("\nLOCAL:\n")
	err = writeImageInfo(logger, localDisplay, localErr)
	if err != nil {
		return fmt.Errorf("writing local builder info: %w", err)
	}

	return nil
}

func writeImageInfo(
	logger logging.Logger,
	info *inspectimage.InfoDisplay,
	err error,
) error {
	imgTpl := template.Must(template.New("runImages").
		Funcs(template.FuncMap{"StringsJoin": strings.Join}).
		Funcs(template.FuncMap{"DashOrValue": dashOrValue}).
		Parse(runImagesTemplate))
	imgTpl = template.Must(imgTpl.New("buildpacks").
		Parse(buildpacksTemplate))
	imgTpl = template.Must(imgTpl.New("processes").
		Parse(processesTemplate))
	imgTpl = template.Must(imgTpl.New("image").
		Parse(imageTemplate))
	if err != nil {
		logger.Errorf("%s\n", err)
		return nil
	}

	if info == nil {
		logger.Info("(not present)\n")
		return nil
	}
	remoteOutput, err := inspectImageOutput(info, imgTpl)
	if err != nil {
		logger.Error(err.Error())
	} else {
		logger.Info(remoteOutput.String())
	}
	return nil
}

func dashOrValue(str string) string {
	if str == "" {
		return "-"
	}

	return str
}

func inspectImageOutput(info *inspectimage.InfoDisplay, tpl *template.Template) (*bytes.Buffer, error) {
	if info == nil {
		return bytes.NewBuffer([]byte("(not present)")), nil
	}
	buf := bytes.NewBuffer(nil)
	tw := tabwriter.NewWriter(buf, 0, 0, 8, ' ', 0)
	defer func() {
		tw.Flush()
	}()
	if err := tpl.Execute(tw, &struct {
		Info *inspectimage.InfoDisplay
	}{
		info,
	}); err != nil {
		return bytes.NewBuffer(nil), err
	}

	return buf, nil
}

var runImagesTemplate = `
Run Images:
{{- range $_, $m := .Info.RunImageMirrors }}
  {{- if $m.UserConfigured }}
  {{$m.Name}}	(user-configured)
  {{- else }}
  {{$m.Name}}
  {{- end }}  
{{- end }}
{{- if not .Info.RunImageMirrors }}
  (none)
{{- end }}`

var buildpacksTemplate = `
Buildpacks:
{{- if .Info.Buildpacks }}
  ID	VERSION	HOMEPAGE
{{- range $_, $b := .Info.Buildpacks }}
  {{ $b.ID }}	{{ $b.Version }}	{{ DashOrValue $b.Homepage }}
{{- end }}
{{- else }}
  (buildpack metadata not present)
{{- end }}`

var processesTemplate = `
{{- if .Info.Processes }}

Processes:
  TYPE	SHELL	COMMAND	ARGS	
  {{- range $_, $p := .Info.Processes }}
    {{- if $p.Default }}
  {{ (printf "%s %s" $p.Type "(default)") }}	{{ $p.Shell }}	{{ $p.Command }}	{{ StringsJoin $p.Args " "  }}	
    {{- else }}
  {{ $p.Type }}	{{ $p.Shell }}	{{ $p.Command }}	{{ StringsJoin $p.Args " " }}	
    {{- end }}
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
{{ template "buildpacks" . }}{{ template "processes" . }}`
