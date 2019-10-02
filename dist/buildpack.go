package dist

import (
	"io"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/api"
	"github.com/buildpack/pack/internal/archive"
	"github.com/buildpack/pack/style"
)

const AssumedBuildpackAPIVersion = "0.1"

type Blob interface {
	Open() (io.ReadCloser, error)
}

type buildpack struct {
	descriptor BuildpackDescriptor
	Blob       `toml:"-"`
}

func (b *buildpack) Descriptor() BuildpackDescriptor {
	return b.descriptor
}

//go:generate mockgen -package testmocks -destination testmocks/buildpack.go github.com/buildpack/pack/dist Buildpack
type Buildpack interface {
	Blob
	Descriptor() BuildpackDescriptor
}

type BuildpackInfo struct {
	ID      string `toml:"id" json:"id"`
	Version string `toml:"version" json:"version,omitempty"`
}

func (b BuildpackInfo) FullName() string {
	if b.Version != "" {
		return b.ID + "@" + b.Version
	}
	return b.ID
}

type Stack struct {
	ID     string
	Mixins []string
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

	bpd.API = api.MustParse(AssumedBuildpackAPIVersion)
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
	if bpd.Info.ID == "" {
		return errors.Errorf("%s is required", style.Symbol("buildpack.id"))
	}

	if bpd.Info.Version == "" {
		return errors.Errorf("%s is required", style.Symbol("buildpack.version"))
	}

	if len(bpd.Order) == 0 && len(bpd.Stacks) == 0 {
		return errors.Errorf(
			"buildpack %s: must have either %s or an %s defined",
			style.Symbol(bpd.Info.FullName()),
			style.Symbol("stacks"),
			style.Symbol("order"),
		)
	}

	if len(bpd.Order) >= 1 && len(bpd.Stacks) >= 1 {
		return errors.Errorf(
			"buildpack %s: cannot have both %s and an %s defined",
			style.Symbol(bpd.Info.FullName()),
			style.Symbol("stacks"),
			style.Symbol("order"),
		)
	}

	return nil
}
