package pack

import (
	"context"

	"github.com/pkg/errors"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/style"
)

type BuilderInfo struct {
	Stack                string
	RunImage             string
	RunImageMirrors      []string
	LocalRunImageMirrors []string
	Buildpacks           []builder.BuildpackMetadata
	Groups               []builder.GroupMetadata
	LifecycleVersion     string
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

	runImageConfig := c.config.GetRunImage(bldr.GetStackInfo().RunImage.Image)

	var localMirrors []string
	if runImageConfig != nil {
		localMirrors = runImageConfig.Mirrors
	}

	return &BuilderInfo{
		Stack:                bldr.StackID,
		RunImage:             bldr.GetStackInfo().RunImage.Image,
		RunImageMirrors:      bldr.GetStackInfo().RunImage.Mirrors,
		LocalRunImageMirrors: localMirrors,
		Buildpacks:           bldr.GetBuildpacks(),
		Groups:               bldr.GetOrder(),
		LifecycleVersion:     bldr.GetLifecycleVersion(),
	}, nil
}
