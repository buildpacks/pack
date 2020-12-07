package inspectimage

import (
	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/launch"

	"github.com/buildpacks/pack"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
)

type GeneralInfo struct {
	Name            string
	RunImageMirrors []config.RunImage
}

type RunImageMirrorDisplay struct {
	Name           string `json:"name" yaml:"name" toml:"name"`
	UserConfigured bool   `json:"user_configured,omitempty" yaml:"user_configured,omitempty" toml:"user_configured,omitempty"`
}

type StackDisplay struct {
	ID     string   `json:"id" yaml:"id" toml:"id"`
	Mixins []string `json:"mixins,omitempty" yaml:"mixins,omitempty" toml:"mixins,omitempty"`
}

type ProcessDisplay struct {
	Type    string   `json:"type" yaml:"type" toml:"type"`
	Shell   string   `json:"shell" yaml:"shell" toml:"shell"`
	Command string   `json:"command" yaml:"command" toml:"command"`
	Default bool     `json:"default" yaml:"default" toml:"default"`
	Args    []string `json:"args" yaml:"args" toml:"args"`
}

type BaseDisplay struct {
	TopLayer  string `json:"top_layer" yaml:"top_layer" toml:"top_layer"`
	Reference string `json:"reference" yaml:"reference" toml:"reference"`
}

type InfoDisplay struct {
	StackID         string                  `json:"stack" yaml:"stack" toml:"stack"`
	Base            BaseDisplay             `json:"base_image" yaml:"base_image" toml:"base_image"`
	RunImageMirrors []RunImageMirrorDisplay `json:"run_images" yaml:"run_images" toml:"run_images"`
	Buildpacks      []dist.BuildpackInfo    `json:"buildpacks" yaml:"buildpacks" toml:"buildpacks"`
	Processes       []ProcessDisplay        `json:"processes" yaml:"processes" toml:"processes"`
}

type InspectOutput struct {
	ImageName string       `json:"image_name" yaml:"image_name" toml:"image_name"`
	Remote    *InfoDisplay `json:"remote_info" yaml:"remote_info" toml:"remote_info"`
	Local     *InfoDisplay `json:"local_info" yaml:"local_info" toml:"local_info"`
}

func NewInfoDisplay(info *pack.ImageInfo, generalInfo GeneralInfo) *InfoDisplay {
	if info == nil {
		return nil
	}
	return &InfoDisplay{
		StackID:         info.StackID,
		Base:            displayBase(info.Base),
		RunImageMirrors: displayMirrors(info, generalInfo),
		Buildpacks:      displayBuildpacks(info.Buildpacks),
		Processes:       displayProcesses(info.Processes),
	}
}

//
// private functions
//

func getConfigMirrors(info *pack.ImageInfo, imageMirrors []config.RunImage) []string {
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

func displayBase(base lifecycle.RunImageMetadata) BaseDisplay {
	return BaseDisplay{
		TopLayer:  base.TopLayer,
		Reference: base.Reference,
	}
}

func displayMirrors(info *pack.ImageInfo, generalInfo GeneralInfo) []RunImageMirrorDisplay {
	// add all user configured run images, then add run images provided by info
	result := []RunImageMirrorDisplay{}
	if info == nil {
		return result
	}

	cfgMirrors := getConfigMirrors(info, generalInfo.RunImageMirrors)
	for _, mirror := range cfgMirrors {
		if mirror != "" {
			result = append(result, RunImageMirrorDisplay{
				Name:           mirror,
				UserConfigured: true,
			})
		}
	}

	// Add run image as named by the stack.
	if info.Stack.RunImage.Image != "" {
		result = append(result, RunImageMirrorDisplay{
			Name:           info.Stack.RunImage.Image,
			UserConfigured: false,
		})
	}

	for _, mirror := range info.Stack.RunImage.Mirrors {
		if mirror != "" {
			result = append(result, RunImageMirrorDisplay{
				Name:           mirror,
				UserConfigured: false,
			})
		}
	}

	return result
}

func displayBuildpacks(buildpacks []lifecycle.Buildpack) []dist.BuildpackInfo {
	result := []dist.BuildpackInfo{}
	for _, buildpack := range buildpacks {
		result = append(result, dist.BuildpackInfo{
			ID:      buildpack.ID,
			Version: buildpack.Version,
		})
	}
	return result
}

func displayProcesses(details pack.ProcessDetails) []ProcessDisplay {
	result := []ProcessDisplay{}
	detailsArray := details.OtherProcesses
	if details.DefaultProcess != nil {
		result = append(result, convertToDisplay(*details.DefaultProcess, true))
	}

	for _, detail := range detailsArray {
		result = append(result, convertToDisplay(detail, false))
	}
	return result
}

func convertToDisplay(proc launch.Process, isDefault bool) ProcessDisplay {
	var shell string
	switch proc.Direct {
	case true:
		shell = ""
	case false:
		shell = "bash"
	}
	result := ProcessDisplay{
		Type:    proc.Type,
		Shell:   shell,
		Command: proc.Command,
		Default: isDefault,
		Args:    proc.Args,
	}

	return result
}
