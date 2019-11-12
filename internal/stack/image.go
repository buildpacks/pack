package stack

import (
	"fmt"
	"sort"
	"strings"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/image"
	"github.com/buildpack/pack/internal/style"
)

const (
	idLabel     = "io.buildpacks.stack.id"
	mixinsLabel = "io.buildpacks.stack.mixins"
)

//go:generate mockgen -package testmocks -destination testmocks/mock_stack_image.go github.com/buildpack/pack/internal/stack Image
type Image interface {
	imgutil.Image
	StackID() string
	Mixins() []string
	CommonMixins() []string
}

// TODO: Test this
type StackImage struct {
	imgutil.Image
	stackID   string
	allMixins []string
}

func NewImage(img imgutil.Image) (*StackImage, error) {
	stackID, ok, err := image.ReadLabel(img, idLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(idLabel), style.Symbol(img.Name()))
	}
	if !ok {
		return nil, fmt.Errorf("image %s missing label %s", style.Symbol(img.Name()), style.Symbol(idLabel))
	}

	var mixins []string
	if _, err := image.UnmarshalLabel(img, mixinsLabel, &mixins); err != nil {
		return nil, err
	}

	return &StackImage{
		Image:     img,
		stackID:   stackID,
		allMixins: mixins,
	}, nil
}

func (s *StackImage) StackID() string {
	return s.stackID
}

func (s *StackImage) Mixins() []string {
	return s.allMixins
}

func (s *StackImage) CommonMixins() []string {
	var mixins []string
	for _, m := range s.allMixins {
		if !strings.HasPrefix(m, "build:") && !strings.HasPrefix(m, "run:") {
			mixins = append(mixins, m)
		}
	}
	return mixins
}

func validateStageMixins(stackImage Image, invalidPrefix string) error {
	var invalid []string
	for _, m := range stackImage.Mixins() {
		if strings.HasPrefix(m, invalidPrefix+":") {
			invalid = append(invalid, m)
		}
	}

	if len(invalid) > 0 {
		sort.Strings(invalid)
		return fmt.Errorf("%s contains %s-only mixin(s): %s", style.Symbol(stackImage.Name()), invalidPrefix, strings.Join(invalid, ", "))
	}
	return nil
}
