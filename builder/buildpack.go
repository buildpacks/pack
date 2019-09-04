package builder

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/style"
)

type buildpack struct {
	descriptor BuildpackDescriptor
	Blob       `toml:"-"`
}

func (b *buildpack) Descriptor() BuildpackDescriptor {
	return b.descriptor
}

type BuildpackDescriptor struct {
	API    *api.Version  `toml:"api"`
	Info   BuildpackInfo `toml:"buildpack"`
	Stacks []Stack       `toml:"stacks"`
	Order  Order         `toml:"order"`
}

//go:generate mockgen -package testmocks -destination testmocks/buildpack.go github.com/buildpack/pack/builder Buildpack
type Buildpack interface {
	Blob
	Descriptor() BuildpackDescriptor
}

type BuildpackInfo struct {
	ID      string `toml:"id" json:"id"`
	Version string `toml:"version" json:"version"`
}

type Stack struct {
	ID string
}

func NewBuildpack(blob Blob) (Buildpack, error) {
	bpd := BuildpackDescriptor{}
	rc, err := blob.Open()
	if err != nil {
		return nil, errors.Wrap(err, "open buildpack")
	}
	defer rc.Close()

	_, buf, err := archive.ReadTarEntry(rc, "buildpack.toml")
	if err != nil {
		return nil, errors.Wrapf(err, "reading buildpack.toml")
	}

	bpd.API = AssumedLifecycleDescriptor().API.BuildpackVersion
	_, err = toml.Decode(string(buf), &bpd)
	if err != nil {
		return nil, errors.Wrapf(err, "decoding buildpack.toml")
	}

	err = validateDescriptor(bpd)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid buildpack.toml")
	}

	return &buildpack{descriptor: bpd, Blob: blob}, nil
}

func validateDescriptor(bpd BuildpackDescriptor) error {
	if len(bpd.Order) == 0 && len(bpd.Stacks) == 0 {
		return fmt.Errorf("buildpack %s must have either stacks or an order defined", style.Symbol(bpd.Info.ID+"@"+bpd.Info.Version))
	}

	if len(bpd.Order) >= 1 && len(bpd.Stacks) >= 1 {
		return fmt.Errorf("buildpack %s cannot have both stacks and an order defined", style.Symbol(bpd.Info.ID+"@"+bpd.Info.Version))
	}

	return nil
}

func (b *BuildpackDescriptor) EscapedID() string {
	return strings.Replace(b.Info.ID, "/", "_", -1)
}

func (b *BuildpackDescriptor) SupportsStack(stackID string) bool {
	for _, stack := range b.Stacks {
		if stack.ID == stackID {
			return true
		}
	}
	return false
}
