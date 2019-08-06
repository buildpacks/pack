package pack

import (
	"context"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/style"
)

type BuilderInfo struct {
	Description      string
	Stack            string
	RunImage         string
	RunImageMirrors  []string
	Buildpacks       []builder.BuildpackMetadata
	Groups           builder.Order
	LifecycleVersion string
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

	var lifecycleVersion string
	if ver := bldr.GetLifecycleVersion(); ver != nil {
		lifecycleVersion = ver.String()
	}

	return &BuilderInfo{
		Description:      bldr.Description(),
		Stack:            bldr.StackID,
		RunImage:         bldr.GetStackInfo().RunImage.Image,
		RunImageMirrors:  bldr.GetStackInfo().RunImage.Mirrors,
		Buildpacks:       bldr.GetBuildpacks(),
		Groups:           bldr.GetOrder(),
		LifecycleVersion: lifecycleVersion,
	}, nil
}
