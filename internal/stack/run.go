package stack

import (
	"strings"
)

type RunImage struct {
	Image
}

func NewRunImage(stackImage Image) (*RunImage, error) {
	run := &RunImage{
		Image: stackImage,
	}

	if err := run.validateStageMixins(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *RunImage) RunOnlyMixins() []string {
	var mixins []string
	for _, m := range r.Mixins() {
		if strings.HasPrefix(m, "run:") {
			mixins = append(mixins, m)
		}
	}
	return mixins
}

func (r *RunImage) validateStageMixins() error {
	return validateStageMixins(r, "build")
}
