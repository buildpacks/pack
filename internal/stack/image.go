package stack

import (
	"fmt"
	"strings"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	image2 "github.com/buildpack/pack/internal/image"
	"github.com/buildpack/pack/internal/style"
)

const (
	idLabel     = "io.buildpacks.stack.id"
	mixinsLabel = "io.buildpacks.stack.mixins"
)

// TODO: Test this
type StackImage struct {
	imgutil.Image
	stackID   string
	allMixins []string
}

func NewImage(img imgutil.Image) (*StackImage, error) {
	stackID, ok, err := image2.ReadLabel(img, idLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(idLabel), style.Symbol(img.Name()))
	}
	if !ok {
		return nil, fmt.Errorf("image %s missing label %s", style.Symbol(img.Name()), style.Symbol(idLabel))
	}

	var mixins []string
	if _, err := image2.UnmarshalLabel(img, mixinsLabel, &mixins); err != nil {
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

// TODO: better name for this method?
func (s *StackImage) CommonMixins() []string {
	var mixins []string
	for _, m := range s.allMixins {
		if !strings.HasPrefix(m, "build:") && !strings.HasPrefix(m, "run:") {
			mixins = append(mixins, m)
		}
	}
	return mixins
}
