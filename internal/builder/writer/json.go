package writer

import (
	"encoding/json"
	"fmt"

	"github.com/buildpacks/pack/internal/style"

	"github.com/buildpacks/pack"
	pubbldr "github.com/buildpacks/pack/builder"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/commands"
	"github.com/buildpacks/pack/internal/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/logging"
)

type InspectOutput struct {
	commands.SharedBuilderInfo
	RemoteInfo *BuilderInfo `json:"remote_info"`
	LocalInfo  *BuilderInfo `json:"local_info"`
}

type RunImage struct {
	Name           string `json:"name"`
	UserConfigured bool   `json:"user_configured,omitempty"`
}

type Lifecycle struct {
	builder.LifecycleInfo
	BuildpackAPIs builder.APIVersions `json:"buildpack_apis"`
	PlatformAPIs  builder.APIVersions `json:"platform_apis"`
}

type Stack struct {
	ID     string   `json:"id"`
	Mixins []string `json:"mixins,omitempty"`
}

type BuilderInfo struct {
	Description            string                  `json:"description,omitempty"`
	CreatedBy              builder.CreatorMetadata `json:"created_by"`
	Stack                  Stack                   `json:"stack"`
	Lifecycle              Lifecycle               `json:"lifecycle"`
	RunImages              []RunImage              `json:"run_images"`
	Buildpacks             []dist.BuildpackInfo    `json:"buildpacks"`
	pubbldr.DetectionOrder `json:"detection_order"`
}

type JSON struct{}

func NewJSON() *JSON {
	return &JSON{}
}

func (w *JSON) Print(
	logger logging.Logger,
	localRunImages []config.RunImage,
	local, remote *pack.BuilderInfo,
	localErr, remoteErr error,
	builderInfo commands.SharedBuilderInfo,
) error {
	if localErr != nil {
		return fmt.Errorf("preparing output for %s: %w", style.Symbol(builderInfo.Name), localErr)
	}

	if remoteErr != nil {
		return fmt.Errorf("preparing output for %s: %w", style.Symbol(builderInfo.Name), remoteErr)
	}

	outputInfo := InspectOutput{SharedBuilderInfo: builderInfo}

	if local != nil {
		stack := Stack{ID: local.Stack}

		if logger.IsVerbose() {
			stack.Mixins = local.Mixins
		}

		outputInfo.LocalInfo = &BuilderInfo{
			Description: local.Description,
			CreatedBy:   local.CreatedBy,
			Stack:       stack,
			Lifecycle: Lifecycle{
				LifecycleInfo: local.Lifecycle.Info,
				BuildpackAPIs: local.Lifecycle.APIs.Buildpack,
				PlatformAPIs:  local.Lifecycle.APIs.Platform,
			},
			RunImages:      runImages(local.RunImage, localRunImages, local.RunImageMirrors),
			Buildpacks:     local.Buildpacks,
			DetectionOrder: local.Order,
		}
	}

	if remote != nil {
		stack := Stack{ID: remote.Stack}

		if logger.IsVerbose() {
			stack.Mixins = remote.Mixins
		}

		outputInfo.RemoteInfo = &BuilderInfo{
			Description: remote.Description,
			CreatedBy:   remote.CreatedBy,
			Stack:       stack,
			Lifecycle: Lifecycle{
				LifecycleInfo: remote.Lifecycle.Info,
				BuildpackAPIs: remote.Lifecycle.APIs.Buildpack,
				PlatformAPIs:  remote.Lifecycle.APIs.Platform,
			},
			RunImages:      runImages(remote.RunImage, localRunImages, remote.RunImageMirrors),
			Buildpacks:     remote.Buildpacks,
			DetectionOrder: remote.Order,
		}
	}

	if outputInfo.LocalInfo == nil && outputInfo.RemoteInfo == nil {
		return fmt.Errorf("unable to find builder %s locally or remotely", style.Symbol(builderInfo.Name))
	}

	var (
		output []byte
		err    error
	)
	if output, err = json.Marshal(outputInfo); err != nil {
		return fmt.Errorf("untested, unexpected failure while marshaling: %w", err)
	}

	logger.Info(string(output))

	return nil
}

func runImages(runImage string, localRunImages []config.RunImage, buildRunImages []string) []RunImage {
	var images = []RunImage{}

	for _, i := range localRunImages {
		if i.Image == runImage {
			for _, m := range i.Mirrors {
				images = append(images, RunImage{Name: m, UserConfigured: true})
			}
		}
	}

	if runImage != "" {
		images = append(images, RunImage{Name: runImage})
	}

	for _, m := range buildRunImages {
		images = append(images, RunImage{Name: m})
	}

	return images
}
