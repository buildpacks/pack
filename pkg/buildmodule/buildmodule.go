package buildmodule

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

//go:generate mockgen -package testmocks -destination ../testmocks/mock_build_module.go github.com/buildpacks/pack/pkg/buildmodule BuildModule

type BuildModule interface {
	// Open returns a reader to a tar with contents structured as per the distribution spec
	// (currently '/cnb/buildpacks/{ID}/{version}/*', all entries with a zeroed-out
	// timestamp and root UID/GID).
	Open() (io.ReadCloser, error)
	Descriptor() Descriptor
}

type Descriptor interface {
	API() *api.Version
	EnsureStackSupport(stackID string, providedMixins []string, validateRunStageMixins bool) error
	EscapedID() string
	Info() dist.ModuleInfo
	Kind() string
	Order() dist.Order
	Stacks() []dist.Stack
}

type Blob interface {
	// Open returns a io.ReadCloser for the contents of the Blob in tar format.
	Open() (io.ReadCloser, error)
}

type buildModule struct {
	descriptor Descriptor
	Blob       `toml:"-"`
}

func (b *buildModule) Descriptor() Descriptor {
	return b.descriptor
}

// FromBlob constructs a buildpack or extension from a blob. It is assumed that the buildpack
// contents are structured as per the distribution spec (currently '/cnb/buildpacks/{ID}/{version}/*' or
// '/cnb/extensions/{ID}/{version}/*').
func FromBlob(descriptor Descriptor, blob Blob) BuildModule {
	return &buildModule{
		Blob:       blob,
		descriptor: descriptor,
	}
}

// FromBuildpackRootBlob constructs a buildpack from a blob. It is assumed that the buildpack contents reside at the
// root of the blob. The constructed buildpack contents will be structured as per the distribution spec (currently
// a tar with contents under '/cnb/buildpacks/{ID}/{version}/*').
func FromBuildpackRootBlob(blob Blob, layerWriterFactory archive.TarWriterFactory) (BuildModule, error) {
	descriptor := dist.BuildpackDescriptor{}
	descriptor.WithAPI = api.MustParse(dist.AssumedBuildpackAPIVersion)
	if err := readDescriptor("buildpack", &descriptor, blob); err != nil {
		return nil, err
	}
	if err := validateBuildpackDescriptor(descriptor); err != nil {
		return nil, err
	}
	return buildpackFrom(&descriptor, blob, layerWriterFactory)
}

// FromExtensionRootBlob constructs an extension from a blob. It is assumed that the extension contents reside at the
// root of the blob. The constructed extension contents will be structured as per the distribution spec (currently
// a tar with contents under '/cnb/extensions/{ID}/{version}/*').
func FromExtensionRootBlob(blob Blob, layerWriterFactory archive.TarWriterFactory) (BuildModule, error) {
	descriptor := dist.ExtensionDescriptor{}
	descriptor.WithAPI = api.MustParse(dist.AssumedBuildpackAPIVersion)
	if err := readDescriptor("extension", &descriptor, blob); err != nil {
		return nil, err
	}
	if err := validateExtensionDescriptor(descriptor); err != nil {
		return nil, err
	}
	return buildpackFrom(&descriptor, blob, layerWriterFactory)
}

func readDescriptor(kind string, descriptor interface{}, blob Blob) error {
	rc, err := blob.Open()
	if err != nil {
		return errors.Wrapf(err, "open %s", kind)
	}
	defer rc.Close()

	descriptorFile := kind + ".toml"

	_, buf, err := archive.ReadTarEntry(rc, descriptorFile)
	if err != nil {
		return errors.Wrapf(err, "reading %s", descriptorFile)
	}

	_, err = toml.Decode(string(buf), descriptor)
	if err != nil {
		return errors.Wrapf(err, "decoding %s", descriptorFile)
	}

	return nil
}

func buildpackFrom(descriptor Descriptor, blob Blob, layerWriterFactory archive.TarWriterFactory) (BuildModule, error) {
	return &buildModule{
		descriptor: descriptor,
		Blob: &distBlob{
			openFn: func() io.ReadCloser {
				return archive.GenerateTarWithWriter(
					func(tw archive.TarWriter) error {
						return toDistTar(tw, descriptor, blob)
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

func toDistTar(tw archive.TarWriter, descriptor Descriptor, blob Blob) error {
	ts := archive.NormalizedDateTime

	parentDir := dist.BuildpacksDir
	if descriptor.Kind() == "extension" {
		parentDir = dist.ExtensionsDir
	}

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     path.Join(parentDir, descriptor.EscapedID()),
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing %s id dir header", descriptor.Kind())
	}

	baseTarDir := path.Join(parentDir, descriptor.EscapedID(), descriptor.Info().Version)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     baseTarDir,
		Mode:     0755,
		ModTime:  ts,
	}); err != nil {
		return errors.Wrapf(err, "writing %s version dir header", descriptor.Kind())
	}

	rc, err := blob.Open()
	if err != nil {
		return errors.Wrapf(err, "reading %s blob", descriptor.Kind())
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

	if len(bpd.Order()) == 0 && len(bpd.Stacks()) == 0 {
		return errors.Errorf(
			"buildpack %s: must have either %s or an %s defined",
			style.Symbol(bpd.Info().FullName()),
			style.Symbol("stacks"),
			style.Symbol("order"),
		)
	}

	if len(bpd.Order()) >= 1 && len(bpd.Stacks()) >= 1 {
		return errors.Errorf(
			"buildpack %s: cannot have both %s and an %s defined",
			style.Symbol(bpd.Info().FullName()),
			style.Symbol("stacks"),
			style.Symbol("order"),
		)
	}

	return nil
}

func validateExtensionDescriptor(extd dist.ExtensionDescriptor) error {
	if extd.Info().ID == "" {
		return errors.Errorf("%s is required", style.Symbol("extension.id"))
	}

	if extd.Info().Version == "" {
		return errors.Errorf("%s is required", style.Symbol("extension.version"))
	}

	return nil
}

func ToLayerTar(dest string, module BuildModule) (string, error) {
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
