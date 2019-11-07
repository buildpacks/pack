package pack

import (
	"context"
	"encoding/json"

	"github.com/Masterminds/semver"
	"github.com/buildpack/lifecycle/metadata"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/image"
)

type ImageInfo struct {
	StackID    string
	Buildpacks []metadata.BuildpackMetadata
	Base       metadata.RunImageMetadata
	BOM        interface{}
	Stack      metadata.StackMetadata
}

func (c *Client) InspectImage(name string, daemon bool) (*ImageInfo, error) {
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, false)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	rawLayersMd, err := metadata.GetRawMetadata(img, metadata.LayerMetadataLabel)
	if err != nil {
		return nil, err
	}
	var layersMd metadata.LayersMetadata
	if rawLayersMd != "" {
		if err := json.Unmarshal([]byte(rawLayersMd), &layersMd); err != nil {
			return nil, errors.Wrapf(err, "failed to parse label '%s'", metadata.LayerMetadataLabel)
		}
	}

	rawBuildMd, _ := metadata.GetRawMetadata(img, metadata.BuildMetadataLabel)
	if err != nil {
		return nil, err
	}
	var buildMD metadata.BuildMetadata
	if rawBuildMd != "" {
		if err := json.Unmarshal([]byte(rawBuildMd), &buildMD); err != nil {
			return nil, errors.Wrapf(err, "failed to parse label '%s'", metadata.BuildMetadataLabel)
		}
	}

	minimumBaseImageReferenceVersion := semver.MustParse("0.5.0")
	actualLauncherVersion, err := semver.NewVersion(buildMD.Launcher.Version)

	if err == nil && actualLauncherVersion.LessThan(minimumBaseImageReferenceVersion) {
		layersMd.RunImage.Reference = ""
	}

	stackID, err := metadata.GetRawMetadata(img, metadata.StackMetadataLabel)
	if err != nil {
		return nil, err
	}

	return &ImageInfo{
		StackID:    stackID,
		Stack:      layersMd.Stack,
		Base:       layersMd.RunImage,
		BOM:        buildMD.BOM,
		Buildpacks: buildMD.Buildpacks,
	}, nil
}
