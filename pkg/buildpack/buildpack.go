package buildpack

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/api"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/dist"
)

type Blob interface {
	// Open returns a io.ReadCloser for the contents of the Blob in tar format.
	Open() (io.ReadCloser, error)
}

//go:generate mockgen -package testmocks -destination ../testmocks/mock_buildpack.go github.com/buildpacks/pack/pkg/buildpack Buildpack

type Buildpack interface { // TODO: this should ideally have a more generic name since it could be a buildpack OR an extension
	// Open returns a reader to a tar with contents structured as per the distribution spec
	// (currently '/cnb/buildpacks/{ID}/{version}/*', all entries with a zeroed-out
	// timestamp and root UID/GID).
	Open() (io.ReadCloser, error)
	Descriptor() dist.BuildpackDescriptor
}

type buildModule struct {
	descriptor dist.BuildpackDescriptor
	Blob       `toml:"-"`
}

func (b *buildModule) Descriptor() dist.BuildpackDescriptor {
	return b.descriptor
}

// FromBlob constructs a buildpack or extension from a blob. It is assumed that the buildpack
// contents are structured as per the distribution spec (currently '/cnb/buildpacks/{ID}/{version}/*' or
// '/cnb/extensions/{ID}/{version}/*').
func FromBlob(bpd dist.BuildpackDescriptor, blob Blob) Buildpack {
	return &buildModule{
		Blob:       blob,
		descriptor: bpd,
	}
}

// FromBuildpackRootBlob constructs a buildpack from a blob. It is assumed that the buildpack contents reside at the
// root of the blob. The constructed buildpack contents will be structured as per the distribution spec (currently
// a tar with contents under '/cnb/buildpacks/{ID}/{version}/*').
func FromBuildpackRootBlob(blob Blob, layerWriterFactory archive.TarWriterFactory) (Buildpack, error) {
	return fromRootBlob("buildpack", blob, layerWriterFactory)
}

// FromExtensionRootBlob constructs an extension from a blob. It is assumed that the extension contents reside at the
// root of the blob. The constructed extension contents will be structured as per the distribution spec (currently
// a tar with contents under '/cnb/extensions/{ID}/{version}/*').
func FromExtensionRootBlob(blob Blob, layerWriterFactory archive.TarWriterFactory) (Buildpack, error) {
	return fromRootBlob("extension", blob, layerWriterFactory)
}

func fromRootBlob(kind string, blob Blob, layerWriterFactory archive.TarWriterFactory) (Buildpack, error) {
	descriptor := dist.BuildpackDescriptor{}
	rc, err := blob.Open()
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", kind)
	}
	defer rc.Close()

	descriptorFile := kind + ".toml"

	_, buf, err := archive.ReadTarEntry(rc, descriptorFile)
	if err != nil {
		return nil, errors.Wrapf(err, "reading %s", descriptorFile)
	}

	descriptor.API = api.MustParse(dist.AssumedBuildpackAPIVersion)
	_, err = toml.Decode(string(buf), &descriptor)
	if err != nil {
		return nil, errors.Wrapf(err, "decoding %s", descriptorFile)
	}

	switch kind {
	case "buildpack":
		err = validateBuildpackDescriptor(descriptor)
	case "extension":
		err = validateExtensionDescriptor(descriptor)
	default:
		return nil, fmt.Errorf("unknown module kind: %s", kind)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "invalid %s", descriptorFile)
	}

	return &buildModule{
		descriptor: descriptor,
		Blob: &distBlob{
			openFn: func() io.ReadCloser {
				return archive.GenerateTarWithWriter(
					func(tw archive.TarWriter) error {
						return toDistTar(kind, tw, descriptor, blob)
					},
					layerWriterFactory,
				)
			},
		},
	}, nil
}

type distBlob struct {
	openFn func() io.ReadCloser
}

func (b *distBlob) Open() (io.ReadCloser, error) {
	return b.openFn(), nil
}

func toDistTar(kind string, tw archive.TarWriter, descriptor dist.BuildpackDescriptor, blob Blob) error {
	ts := archive.NormalizedDateTime

	var parentDir string
	switch kind {
	case "buildpack":
		parentDir = dist.BuildpacksDir
	case "extension":
		parentDir = dist.ExtensionsDir
	default:
		return fmt.Errorf("unknown module kind: %s", kind)
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join(parentDir, descriptor.EscapedID()),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing %s id dir header", kind)
	}

	baseTarDir := path.Join(parentDir, descriptor.EscapedID(), descriptor.Info().Version)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     baseTarDir,
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing %s version dir header", kind)
	}

	rc, err := blob.Open()
	if err != nil {
		return errors.Wrapf(err, "reading %s blob", kind)
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "failed to get next tar entry")
		}

		archive.NormalizeHeader(header, true)
		header.Name = path.Clean(header.Name)
		if header.Name == "." || header.Name == "/" {
			continue
		}

		header.Mode = calcFileMode(header)
		header.Name = path.Join(baseTarDir, header.Name)
		err = tw.WriteHeader(header)
		if err != nil {
			return errors.Wrapf(err, "failed to write header for '%s'", header.Name)
		}

		_, err = io.Copy(tw, tr)
		if err != nil {
			return errors.Wrapf(err, "failed to write contents to '%s'", header.Name)
		}
	}

	return nil
}

func calcFileMode(header *tar.Header) int64 {
	switch {
	case header.Typeflag == tar.TypeDir:
		return 0755
	case nameOneOf(header.Name,
		path.Join("bin", "detect"),
		path.Join("bin", "build"),
	):
		return 0755
	case anyExecBit(header.Mode):
		return 0755
	}

	return 0644
}

func nameOneOf(name string, paths ...string) bool {
	for _, p := range paths {
		if name == p {
			return true
		}
	}
	return false
}

func anyExecBit(mode int64) bool {
	return mode&0111 != 0
}

func validateBuildpackDescriptor(bpd dist.BuildpackDescriptor) error {
	if bpd.Info().ID == "" {
		return errors.Errorf("%s is required", style.Symbol("buildpack.id"))
	}

	if bpd.Info().Version == "" {
		return errors.Errorf("%s is required", style.Symbol("buildpack.version"))
	}

	if len(bpd.Order) == 0 && len(bpd.Stacks) == 0 {
		return errors.Errorf(
			"buildpack %s: must have either %s or an %s defined",
			style.Symbol(bpd.Info().FullName()),
			style.Symbol("stacks"),
			style.Symbol("order"),
		)
	}

	if len(bpd.Order) >= 1 && len(bpd.Stacks) >= 1 {
		return errors.Errorf(
			"buildpack %s: cannot have both %s and an %s defined",
			style.Symbol(bpd.Info().FullName()),
			style.Symbol("stacks"),
			style.Symbol("order"),
		)
	}

	if bpd.ExtInfo.ID != "" {
		return errors.Errorf(
			"buildpack %s: cannot have %s defined",
			style.Symbol(bpd.Info().FullName()),
			style.Symbol("extension"),
		)
	}

	return nil
}

func validateExtensionDescriptor(extd dist.BuildpackDescriptor) error {
	if extd.Info().ID == "" {
		return errors.Errorf("%s is required", style.Symbol("extension.id"))
	}

	if extd.Info().Version == "" {
		return errors.Errorf("%s is required", style.Symbol("extension.version"))
	}

	if len(extd.Order) >= 1 {
		return errors.Errorf(
			"extension %s: cannot have %s defined",
			style.Symbol(extd.Info().FullName()),
			style.Symbol("stacks"),
		)
	}

	if extd.BpInfo.ID != "" {
		return errors.Errorf(
			"extension %s: cannot have %s defined",
			style.Symbol(extd.Info().FullName()),
			style.Symbol("buildpack"),
		)
	}

	return nil
}

func ToLayerTar(dest string, module Buildpack) (string, error) {
	descriptor := module.Descriptor()
	modReader, err := module.Open()
	if err != nil {
		return "", errors.Wrap(err, "opening blob")
	}
	defer modReader.Close()

	layerTar := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", descriptor.EscapedID(), descriptor.Info().Version))
	fh, err := os.Create(layerTar)
	if err != nil {
		return "", errors.Wrap(err, "create file for tar")
	}
	defer fh.Close()

	if _, err := io.Copy(fh, modReader); err != nil {
		return "", errors.Wrap(err, "writing blob to tar")
	}

	return layerTar, nil
}
