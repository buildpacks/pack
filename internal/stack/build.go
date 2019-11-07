package stack

import (
	"strings"

	"github.com/buildpack/imgutil"
)

// TODO: Test this
type BuildImage struct {
	StackImage
}

func NewBuildImage(raw imgutil.Image) (*BuildImage, error) {
	image, err := NewImage(raw)
	if err != nil {
		return nil, err
	}

	build := &BuildImage{
		StackImage: *image,
	}

	if err := validateStageMixins(build, false); err != nil {
		return nil, err
	}

	return build, nil
}

func (b *BuildImage) BuildOnlyMixins() []string {
	var mixins []string
	for _, m := range b.allMixins {
		if strings.HasPrefix(m, "build:") {
			mixins = append(mixins)
		}
	}
	return mixins
}
