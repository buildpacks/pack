package pack

import (
	"context"
	"github.com/buildpack/pack/style"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/image"

	"github.com/buildpack/pack/builder"
)

type BuilderInfo struct {
	Stack                string
	RunImage             string
	RunImageMirrors      []string
	LocalRunImageMirrors []string
	Buildpacks           []BuildpackInfo
	Groups               [][]BuildpackInfo
}

type BuildpackInfo struct {
	ID      string
	Version string
	Latest  bool
}

func (c *Client) InspectBuilder(name string, daemon bool) (*BuilderInfo, error) {
	img, err := c.fetcher.Fetch(context.Background(), name, daemon, false)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	bldr := builder.NewBuilder(img, c.config)

	stackID, err := bldr.GetStack()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get stack ID for builder image %s", style.Symbol(name))
	}

	metadata, err := bldr.GetMetadata()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get metadata for builder image %s", style.Symbol(name))
	}

	localMirrors, err := bldr.GetLocalRunImageMirrors()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local run image mirrors for builder image %s", style.Symbol(name))
	}

	var buildpacks []BuildpackInfo
	for _, bp := range metadata.Buildpacks {
		buildpacks = append(buildpacks, buildpackMetadataToInfo(bp))
	}

	groups := make([][]BuildpackInfo, len(metadata.Groups))
	for i, group := range metadata.Groups {
		for _, bp := range group.Buildpacks {
			groups[i] = append(groups[i], buildpackMetadataToInfo(bp))
		}
	}

	return &BuilderInfo{
		Stack:                stackID,
		RunImage:             metadata.Stack.RunImage.Image,
		RunImageMirrors:      metadata.Stack.RunImage.Mirrors,
		LocalRunImageMirrors: localMirrors,
		Buildpacks:           buildpacks,
		Groups:               groups,
	}, nil
}

func buildpackMetadataToInfo(bp builder.BuildpackMetadata) BuildpackInfo {
	return BuildpackInfo{
		ID:      bp.ID,
		Version: bp.Version,
		Latest:  bp.Latest,
	}
}
