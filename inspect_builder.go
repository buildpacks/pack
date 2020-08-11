package pack

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/style"
)

// BuilderInfo is a collection of metadata describing an builder created using pack.
type BuilderInfo struct {
	// Description of builder
	Description     string

	// stack name use by the builder
	Stack           string

	// list of mixins provided by the Stack
	Mixins          []string

	RunImage        string

	// list of all run image mirrors a builder will use to provide
	// the RunImage.
	RunImageMirrors []string

	// All buildpacks included within the builder.
	Buildpacks      []dist.BuildpackInfo

	// Top level ordering of buildpacks.
	Order           dist.Order

	// metadata about the lifecycle Version this builder uses
	// as well which platform API, and buildpack API this builder
	// implements
	Lifecycle       builder.LifecycleDescriptor

	// Name and Version information from the tooling used
	// to create this builder.
	CreatedBy       builder.CreatorMetadata
}

// InspectBuilder reads label metadata of a local builder image. Initializes a BuilderInfo
// object with this metadata, and returns it.
func (c *Client) InspectBuilder(name string, daemon bool) (*BuilderInfo, error) {
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, false)
	if err != nil {
		if errors.Cause(err) == image.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	bldr, err := builder.FromImage(img)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid builder %s", style.Symbol(name))
	}

	var commonMixins, buildMixins []string
	commonMixins = []string{}
	for _, mixin := range bldr.Mixins() {
		if strings.HasPrefix(mixin, "build:") {
			buildMixins = append(buildMixins, mixin)
		} else {
			commonMixins = append(commonMixins, mixin)
		}
	}

	return &BuilderInfo{
		Description:     bldr.Description(),
		Stack:           bldr.StackID,
		Mixins:          append(commonMixins, buildMixins...),
		RunImage:        bldr.Stack().RunImage.Image,
		RunImageMirrors: bldr.Stack().RunImage.Mirrors,
		Buildpacks:      bldr.Buildpacks(),
		Order:           bldr.Order(),
		Lifecycle:       bldr.LifecycleDescriptor(),
		CreatedBy:       bldr.CreatedBy(),
	}, nil
}
