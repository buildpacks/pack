package pack

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/image"
	"github.com/buildpacks/pack/internal/style"
)

// BuilderInfo is a collection of metadata describing a builder created using pack.
type BuilderInfo struct {
	// Human readable, description of a builder.
	Description string

	// Stack name used by the builder.
	Stack string

	// List of Stack mixins, this information is provided by Stack variable.
	Mixins []string

	// RunImage provided by the builder.
	RunImage string

	// List of all run image mirrors a builder will use to provide
	// the RunImage.
	RunImageMirrors []string

	// All buildpacks included within the builder.
	Buildpacks []dist.BuildpackInfo

	// Top level ordering of buildpacks.
	Order dist.Order

	// Listing of all buildpack layers in a builder.
	// All elements in the Buildpacks variable are represented in this
	// object.
	BuildpackLayers dist.BuildpackLayers

	// Lifecycle provides the following API versioning information for a builder:
	// - Lifecycle Version used in this builder,
	// - Platform API,
	// - Buildpack API.
	Lifecycle builder.LifecycleDescriptor

	// Name and Version information from tooling used
	// to produce this builder.
	CreatedBy builder.CreatorMetadata
}

// BuildpackInfoKey contains all information needed to determine
// buildpack equivalence.
type BuildpackInfoKey struct {
	ID      string
	Version string
}

// InspectBuilder reads label metadata of a local or remote builder image. It initializes a BuilderInfo
// object with this metadata, and returns it. This method will error if the name image cannot be found
// both locally and remotely, or if the found image does not contain the proper labels.
func (c *Client) InspectBuilder(name string, daemon bool) (*BuilderInfo, error) {
	img, err := c.imageFetcher.Fetch(context.Background(), name, daemon, config.PullNever)
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

	var bpLayers dist.BuildpackLayers
	if _, err := dist.GetLabel(img, dist.BuildpackLayersLabel, &bpLayers); err != nil {
		return nil, err
	}

	return &BuilderInfo{
		Description:     bldr.Description(),
		Stack:           bldr.StackID,
		Mixins:          append(commonMixins, buildMixins...),
		RunImage:        bldr.Stack().RunImage.Image,
		RunImageMirrors: bldr.Stack().RunImage.Mirrors,
		Buildpacks:      uniqueBuildpacks(bldr.Buildpacks()),
		Order:           bldr.Order(),
		BuildpackLayers: bpLayers,
		Lifecycle:       bldr.LifecycleDescriptor(),
		CreatedBy:       bldr.CreatedBy(),
	}, nil
}

func uniqueBuildpacks(buildpacks []dist.BuildpackInfo) []dist.BuildpackInfo {
	buildpacksSet := map[BuildpackInfoKey]int{}
	homePageSet := map[BuildpackInfoKey]string{}
	for _, buildpack := range buildpacks {
		key := BuildpackInfoKey{
			ID:      buildpack.ID,
			Version: buildpack.Version,
		}
		_, ok := buildpacksSet[key]
		if !ok {
			buildpacksSet[key] = len(buildpacksSet)
			homePageSet[key] = buildpack.Homepage
		}
	}
	result := make([]dist.BuildpackInfo, len(buildpacksSet))
	for buildpackKey, index := range buildpacksSet {
		result[index] = dist.BuildpackInfo{
			ID:       buildpackKey.ID,
			Version:  buildpackKey.Version,
			Homepage: homePageSet[buildpackKey],
		}
	}

	return result
}
