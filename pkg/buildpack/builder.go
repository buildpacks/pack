package buildpack

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"

	"github.com/buildpacks/imgutil/layer"

	"github.com/buildpacks/imgutil"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/dist"
)

type ImageFactory interface {
	NewImage(repoName string, local bool, imageOS string) (imgutil.Image, error)
}

type WorkableImage interface {
	SetLabel(string, string) error
	AddLayerWithDiffID(path, diffID string) error
}

type layoutImage struct {
	v1.Image
}

func (i *layoutImage) SetLabel(key string, val string) error {
	configFile, err := i.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Labels[key] = val
	i.Image, err = mutate.Config(i.Image, config)
	return err
}

func (i *layoutImage) AddLayerWithDiffID(path, _ string) error {
	tarLayer, err := tarball.LayerFromFile(path, tarball.WithCompressionLevel(gzip.DefaultCompression))
	if err != nil {
		return err
	}
	i.Image, err = mutate.AppendLayers(i.Image, tarLayer)
	if err != nil {
		return errors.Wrap(err, "add layer")
	}
	return nil
}

type PackageBuilder struct {
	buildpack    BuildModule
	dependencies []BuildModule
	imageFactory ImageFactory
}

// TODO: Rename to PackageBuilder
func NewBuilder(imageFactory ImageFactory) *PackageBuilder {
	return &PackageBuilder{
		imageFactory: imageFactory,
	}
}

func (b *PackageBuilder) SetBuildpack(buildpack BuildModule) {
	b.buildpack = buildpack
}

func (b *PackageBuilder) AddDependency(buildpack BuildModule) {
	b.dependencies = append(b.dependencies, buildpack)
}

func (b *PackageBuilder) finalizeImage(image WorkableImage, tmpDir string) error {
	if err := dist.SetLabel(image, MetadataLabel, &Metadata{
		ModuleInfo: b.buildpack.Descriptor().ModuleInfo(),
		Stacks:     b.resolvedStacks(),
	}); err != nil {
		return err
	}

	bpLayers := dist.BuildpackLayers{}
	for _, bp := range append(b.dependencies, b.buildpack) {
		bpLayerTar, err := ToLayerTar(tmpDir, bp)
		if err != nil {
			return err
		}

		diffID, err := dist.LayerDiffID(bpLayerTar)
		if err != nil {
			return errors.Wrapf(err,
				"getting content hashes for buildpack %s",
				style.Symbol(bp.Descriptor().ModuleInfo().FullName()),
			)
		}

		if err := image.AddLayerWithDiffID(bpLayerTar, diffID.String()); err != nil {
			return errors.Wrapf(err, "adding layer tar for buildpack %s", style.Symbol(bp.Descriptor().ModuleInfo().FullName()))
		}

		dist.AddToLayersMD(bpLayers, bp.Descriptor(), diffID.String())
	}

	if err := dist.SetLabel(image, dist.BuildpackLayersLabel, bpLayers); err != nil {
		return err
	}

	return nil
}

func (b *PackageBuilder) validate() error {
	if b.buildpack == nil {
		return errors.New("buildpack must be set")
	}

	if err := validateBuildpacks(b.buildpack, b.dependencies); err != nil {
		return err
	}

	if len(b.resolvedStacks()) == 0 {
		return errors.Errorf("no compatible stacks among provided buildpacks")
	}

	return nil
}

func (b *PackageBuilder) resolvedStacks() []dist.Stack {
	stacks := b.buildpack.Descriptor().ModuleStacks()
	for _, bp := range b.dependencies {
		bpd := bp.Descriptor()

		if len(stacks) == 0 {
			stacks = bpd.ModuleStacks()
		} else if len(bpd.ModuleStacks()) > 0 { // skip over "meta-buildpacks"
			stacks = stack.MergeCompatible(stacks, bpd.ModuleStacks())
		}
	}

	return stacks
}

func (b *PackageBuilder) SaveAsFile(path, imageOS string) error {
	if err := b.validate(); err != nil {
		return err
	}

	layoutImage, err := newLayoutImage(imageOS)
	if err != nil {
		return errors.Wrap(err, "creating layout image")
	}

	tmpDir, err := ioutil.TempDir("", "package-buildpack")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := b.finalizeImage(layoutImage, tmpDir); err != nil {
		return err
	}

	layoutDir, err := ioutil.TempDir(tmpDir, "oci-layout")
	if err != nil {
		return errors.Wrap(err, "creating oci-layout temp dir")
	}

	p, err := layout.Write(layoutDir, empty.Index)
	if err != nil {
		return errors.Wrap(err, "writing index")
	}

	if err := p.AppendImage(layoutImage); err != nil {
		return errors.Wrap(err, "writing layout")
	}

	outputFile, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "creating output file")
	}
	defer outputFile.Close()

	tw := tar.NewWriter(outputFile)
	defer tw.Close()

	return archive.WriteDirToTar(tw, layoutDir, "/", 0, 0, 0755, true, false, nil)
}

func newLayoutImage(imageOS string) (*layoutImage, error) {
	i := empty.Image

	configFile, err := i.ConfigFile()
	if err != nil {
		return nil, err
	}

	configFile.OS = imageOS
	i, err = mutate.ConfigFile(i, configFile)
	if err != nil {
		return nil, err
	}

	if imageOS == "windows" {
		opener := func() (io.ReadCloser, error) {
			reader, err := layer.WindowsBaseLayer()
			return io.NopCloser(reader), err
		}

		baseLayer, err := tarball.LayerFromOpener(opener, tarball.WithCompressionLevel(gzip.DefaultCompression))
		if err != nil {
			return nil, err
		}

		i, err = mutate.AppendLayers(i, baseLayer)
		if err != nil {
			return nil, err
		}
	}

	return &layoutImage{Image: i}, nil
}

func (b *PackageBuilder) SaveAsImage(repoName string, publish bool, imageOS string) (imgutil.Image, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	image, err := b.imageFactory.NewImage(repoName, !publish, imageOS)
	if err != nil {
		return nil, errors.Wrapf(err, "creating image")
	}

	tmpDir, err := ioutil.TempDir("", "package-buildpack")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	if err := b.finalizeImage(image, tmpDir); err != nil {
		return nil, err
	}

	if err := image.Save(); err != nil {
		return nil, err
	}

	return image, nil
}

func validateBuildpacks(mainBP BuildModule, depBPs []BuildModule) error {
	depsWithRefs := map[string][]dist.ModuleInfo{}

	for _, bp := range depBPs {
		depsWithRefs[bp.Descriptor().ModuleInfo().FullName()] = nil
	}

	for _, bp := range append([]BuildModule{mainBP}, depBPs...) { // List of everything
		bpd := bp.Descriptor()
		for _, orderEntry := range bpd.ModuleOrder() {
			for _, groupEntry := range orderEntry.Group {
				bpFullName, err := groupEntry.ModuleInfo.FullNameWithVersion()
				if err != nil {
					return errors.Wrapf(
						err,
						"buildpack %s must specify a version when referencing buildpack %s",
						style.Symbol(bpd.ModuleInfo().FullName()),
						style.Symbol(bpFullName),
					)
				}
				if _, ok := depsWithRefs[bpFullName]; !ok {
					return errors.Errorf(
						"buildpack %s references buildpack %s which is not present",
						style.Symbol(bpd.ModuleInfo().FullName()),
						style.Symbol(bpFullName),
					)
				}

				depsWithRefs[bpFullName] = append(depsWithRefs[bpFullName], bpd.ModuleInfo())
			}
		}
	}

	for bp, refs := range depsWithRefs {
		if len(refs) == 0 {
			return errors.Errorf(
				"buildpack %s is not used by buildpack %s",
				style.Symbol(bp),
				style.Symbol(mainBP.Descriptor().ModuleInfo().FullName()),
			)
		}
	}

	return nil
}
