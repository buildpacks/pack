package pack

import (
	"context"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/style"
)

type BuilderInfo struct {
	Description     string
	Stack           string
	RunImage        string
	RunImageMirrors []string
	Buildpacks      []builder.BuildpackMetadata
	Order           dist.Order
	Lifecycle       builder.LifecycleDescriptor
	CreatedBy       builder.CreatorMetadata
}

type BuildpackInfo struct {
	ID      string
	Version string
	Latest  bool
}

func (c *Client) InspectBuilder(name string, daemon bool) (*BuilderInfo, error) {
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, false)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	bldr, err := builder.GetBuilder(img)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid builder %s", style.Symbol(name))
	}

	return &BuilderInfo{
		Description:     bldr.Description(),
		Stack:           bldr.StackID,
		RunImage:        bldr.GetStackInfo().RunImage.Image,
		RunImageMirrors: bldr.GetStackInfo().RunImage.Mirrors,
		Buildpacks:      bldr.GetBuildpacks(),
		Order:           bldr.GetOrder(),
		Lifecycle:       bldr.GetLifecycleDescriptor(),
		CreatedBy:       bldr.GetCreatedBy(),
	}, nil
}
