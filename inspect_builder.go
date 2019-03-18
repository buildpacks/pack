package pack

import (
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/pack/builder"
	"github.com/pkg/errors"
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
	var (
		img image.Image
		err error
	)

	if daemon {
		img, err = c.fetcher.FetchLocalImage(name)
	} else {
		img, err = c.fetcher.FetchRemoteImage(name)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get builder image '%s'", name)
	}

	if found, err := img.Found(); err != nil {
		return nil, errors.Wrapf(err, "failed to find builder image '%s'", name)
	} else if !found {
		return nil, nil
	}

	bldr := builder.NewBuilder(img, c.config)

	stack, err := bldr.GetStack()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get stack for builder image '%s'", name)
	}

	metadata, err := bldr.GetMetadata()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get metadata for builder image '%s'", name)
	}

	localMirrors, err := bldr.GetLocalRunImageMirrors()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get local run image mirrors for builder image '%s'", name)
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
		Stack:                stack,
		RunImage:             metadata.RunImage.Image,
		RunImageMirrors:      metadata.RunImage.Mirrors,
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
