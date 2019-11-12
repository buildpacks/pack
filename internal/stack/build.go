package stack

import (
	"strings"
)

// TODO: Test this
type BuildImage struct {
	Image
}

func NewBuildImage(stackImage Image) (*BuildImage, error) {
	build := &BuildImage{
		Image: stackImage,
	}

	if err := build.validateStageMixins(); err != nil {
		return nil, err
	}

	return build, nil
}

func (b *BuildImage) BuildOnlyMixins() []string {
	var mixins []string
	for _, m := range b.Mixins() {
		if strings.HasPrefix(m, "build:") {
			mixins = append(mixins)
		}
	}
	return mixins
}

func (b *BuildImage) validateStageMixins() error {
	return validateStageMixins(b, "run")
}