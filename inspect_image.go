package pack

import (
	"context"

	"github.com/buildpacks/pack/config"

	"github.com/Masterminds/semver"
	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
)

// ImageInfo is a collection of metadata describing
// an app image built using Cloud Native Buildpacks.
type ImageInfo struct {
	// Stack Identifier used when building this image
	StackID string

	// List of buildpacks that passed detection, ran their build
	// phases and made a contribution to this image.
	Buildpacks []lifecycle.Buildpack

	// Base includes two references to the run image,
	// - the Run Image ID,
	// - the hash of the last layer in the app image that belongs to the run image.
	// A way to visualize this is given an image with n layers:
	//
	// last layer in run image
	//          v
	// [1, ..., k, k+1, ..., n]
	//              ^
	//   first layer added by buildpacks
	//
	// the first 1 to k layers all belong to the run image,
	// the last k+1, to n are layers added by buildpacks.
	// the sum of all of these is our app image.
	Base lifecycle.RunImageMetadata

	// BOM or Bill of materials, contains dependency and
	// version information provided by each buildpack.
	BOM []lifecycle.BOMEntry

	// Stack includes the run image name, and a list of image mirrors,
	// where the run image is hosted.
	Stack lifecycle.StackMetadata

	// Processes lists all processes contributed by buildpacks.
	Processes ProcessDetails
}

// ProcessDetails is a collection of all start command metadata
// on an image
type ProcessDetails struct {
	// images default start command
	DefaultProcess *launch.Process

	// list of all start commands contributed by buildpacks.
	OtherProcesses []launch.Process
}

// Deserialize just the subset of fields we need to avoid breaking changes
type layersMetadata struct {
	RunImage lifecycle.RunImageMetadata `json:"runImage" toml:"run-image"`
	Stack    lifecycle.StackMetadata    `json:"stack" toml:"stack"`
}

// InspectImage reads the Label metadata of an image. Initializes a ImageInfo object
// using this metadata, and returns it.
// If daemon is true, the local registry will be searched first for the image.
// Otherwise assume the image is remote.
func (c *Client) InspectImage(name string, daemon bool) (*ImageInfo, error) {
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, config.PullNever)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	var layersMd layersMetadata
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
