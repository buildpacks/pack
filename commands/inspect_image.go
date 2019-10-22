package commands

import (
	"bytes"
	"encoding/json"
	"text/tabwriter"
	"text/template"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/style"

	"github.com/buildpack/pack/logging"
)

type InspectImageFlags struct {
	BOM bool
}

func InspectImage(logger logging.Logger, cfg *config.Config, client PackClient) *cobra.Command {
	var flags InspectImageFlags
	cmd := &cobra.Command{
		Use:   "inspect-image <image-name>",
		Short: "Show information about a built image",
		Args:  cobra.ExactArgs(1),
		RunE: logError(logger, func(cmd *cobra.Command, args []string) error {
			img := args[0]
			remote, err := client.InspectImage(img, false)
			if err != nil {
				logger.Errorf("inspecting remote image '%s': %s", img, err)
			}
			local, err := client.InspectImage(img, true)
			if err != nil {
				logger.Errorf("inspecting local image '%s': %s", img, err)
			}
			if flags.BOM {
				return logBOM(remote, local, logger)
			}
			logger.Infof("Inspecting image: %s\n", style.Symbol(img))
			logDetails(remote, local, *cfg, logger)
			return nil
		}),
	}
	AddHelpFlag(cmd, "inspect-image")
	cmd.Flags().BoolVar(&flags.BOM, "bom", false, "print bill of materials")
	return cmd
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

func logDetails(remote *pack.ImageInfo, local *pack.ImageInfo, cfg config.Config, logger logging.Logger) {
	imgTpl := template.Must(template.New("runImages").Parse(runImagesTemplate))
	imgTpl = template.Must(imgTpl.New("buildpacks").Parse(buildpacksTemplate))
	imgTpl = template.Must(imgTpl.New("image").Parse(imageTemplate))
	remoteOutput, err := inspectImageOutput(remote, cfg, imgTpl)
	if err != nil {
		logger.Error(err.Error())
	} else {
		logger.Infof("\nREMOTE:\n%s", remoteOutput)
	}
	localOutput, err := inspectImageOutput(local, cfg, imgTpl)
	if err != nil {
		logger.Error(err.Error())
	} else {
		logger.Infof("\nLOCAL:\n%s", localOutput)
	}
}

func inspectImageOutput(
	info *pack.ImageInfo,
	cfg config.Config,
	tpl *template.Template,
) (output string, err error) {
	if info == nil {
		return "(not present)", nil
	}
	var buf bytes.Buffer
	localMirrors := getLocalMirrors(info.Stack.RunImage.Image, cfg)
	tw := tabwriter.NewWriter(&buf, 0, 0, 8, ' ', 0)
	defer tw.Flush()
	if err := tpl.Execute(tw, &struct {
		Info         *pack.ImageInfo
		LocalMirrors []string
	}{
		info,
		localMirrors,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
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
{{- end }}
`

var imageTemplate = `
Stack: {{ .Info.StackID }}

Base Image:
{{- if .Info.Base.Reference}}
  Reference: {{ .Info.Base.Reference }}
{{- end}}
  Top Layer: {{ .Info.Base.TopLayer }}
{{ template "runImages" . }}
{{ template "buildpacks" . }}
`
