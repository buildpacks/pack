package stack

import (
	"strings"

	"github.com/buildpack/imgutil"
)

// TODO: Test this
type runImage struct {
	StackImage // TODO: should this be `*stackImage` instead?
}

type runOnlyMixiner interface {
	RunOnlyMixins() []string
}

func NewRunImage(raw imgutil.Image) (*runImage, error) {
	image, err := NewImage(raw)
	if err != nil {
		return nil, err
	}

	run := &runImage{
		StackImage: *image,
	}

	if err := validateStageMixins(run, true); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *runImage) RunOnlyMixins() []string {
	var mixins []string
	for _, m := range r.allMixins {
		if strings.HasPrefix(m, "run:") {
			mixins = append(mixins)
		}
	}
	return mixins
}

func (r *runImage) Validate(img runOnlyMixiner) error {
	return nil
}
