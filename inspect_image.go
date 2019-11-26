package pack

import (
	"context"

	"github.com/Masterminds/semver"
	"github.com/buildpack/lifecycle"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/image"
)

type ImageInfo struct {
	StackID    string
	Buildpacks []lifecycle.Buildpack
	Base       lifecycle.RunImageMetadata
	BOM        []lifecycle.BOMEntry
	Stack      lifecycle.StackMetadata
	Processes  ProcessDetails
}

type ProcessDetails struct {
	DefaultProcess *lifecycle.Process
	OtherProcesses []lifecycle.Process
}

func (c *Client) InspectImage(name string, daemon bool) (*ImageInfo, error) {
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, false)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	var layersMd lifecycle.LayersMetadata
	if _, err := dist.GetLabel(img, lifecycle.LayerMetadataLabel, &layersMd); err != nil {
		return nil, err
	}

	var buildMD lifecycle.BuildMetadata
	if _, err := dist.GetLabel(img, lifecycle.BuildMetadataLabel, &buildMD); err != nil {
		return nil, err
	}

	minimumBaseImageReferenceVersion := semver.MustParse("0.5.0")
	actualLauncherVersion, err := semver.NewVersion(buildMD.Launcher.Version)

	if err == nil && actualLauncherVersion.LessThan(minimumBaseImageReferenceVersion) {
		layersMd.RunImage.Reference = ""
	}

	stackID, err := img.Label(lifecycle.StackIDLabel)
	if err != nil {
		return nil, err
	}

	defaultProcessType, err := img.Env("CNB_PROCESS_TYPE")
	if err != nil || defaultProcessType == "" {
		defaultProcessType = "web"
	}

	var processDetails ProcessDetails
	for _, proc := range buildMD.Processes {
		proc := proc
		if proc.Type == defaultProcessType {
			processDetails.DefaultProcess = &proc
			continue
		}
		processDetails.OtherProcesses = append(processDetails.OtherProcesses, proc)
	}

	return &ImageInfo{
		StackID:    stackID,
		Stack:      layersMd.Stack,
		Base:       layersMd.RunImage,
		BOM:        buildMD.BOM,
		Buildpacks: buildMD.Buildpacks,
		Processes:  processDetails,
	}, nil
}
