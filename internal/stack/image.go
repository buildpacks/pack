package stack

import (
	"fmt"
	"strings"

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
	ReadableImage
	StackID() string
	Mixins() []string
	CommonMixins() []string
}

type ReadableImage interface {
	Name() string
	Label(string) (string, error)
	Env(string) (string, error)
}

type stackImage struct {
	allMixins []string
	image     ReadableImage // Deprecated: should store values in constructor
	name      string
	stackID   string
}

func NewImage(img ReadableImage) (Image, error) {
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

	return &stackImage{
		image:     img,
		name:      img.Name(),
		stackID:   stackID,
		allMixins: mixins,
	}, nil
}

func (s *stackImage) Name() string {
	return s.name
}

func (s *stackImage) Label(name string) (string, error) {
	return s.image.Label(name)
}

func (s *stackImage) Env(name string) (string, error) {
	return s.image.Env(name)
}

func (s *stackImage) StackID() string {
	return s.stackID
}

func (s *stackImage) Mixins() []string {
	return s.allMixins
}

func (s *stackImage) CommonMixins() []string {
	var mixins []string
	for _, m := range s.allMixins {
		if !strings.HasPrefix(m, "build:") && !strings.HasPrefix(m, "run:") {
			mixins = append(mixins, m)
		}
	}
	return mixins
}
