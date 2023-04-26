package buildpack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layer"
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

type toAdd struct {
	tarPath string
	diffID  string
	module  BuildModule
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

type PackageBuilderOption func(*options) error

type options struct {
	flatten bool
	depth   int
	exclude []string
}

type PackageBuilder struct {
	buildpack                BuildModule
	extension                BuildModule
	dependencies             ModuleManager
	imageFactory             ImageFactory
	flattenAllBuildpacks     bool
	flattenExcludeBuildpacks []string
}

// TODO: Rename to PackageBuilder
func NewBuilder(imageFactory ImageFactory, ops ...PackageBuilderOption) *PackageBuilder {
	opts := &options{}
	for _, op := range ops {
		if err := op(opts); err != nil {
			return nil
		}
	}
	moduleManager := NewModuleManager(opts.flatten, opts.depth)
	return &PackageBuilder{
		imageFactory:             imageFactory,
		dependencies:             *moduleManager,
		flattenAllBuildpacks:     opts.flatten && opts.depth < 0,
		flattenExcludeBuildpacks: opts.exclude,
	}
}

func WithFlatten(depth int, exclude []string) PackageBuilderOption {
	return func(o *options) error {
		o.flatten = true
		o.depth = depth
		o.exclude = exclude
		return nil
	}
}

func (b *PackageBuilder) SetBuildpack(buildpack BuildModule) {
	b.buildpack = buildpack
}
func (b *PackageBuilder) SetExtension(extension BuildModule) {
	b.extension = extension
}

func (b *PackageBuilder) AddDependency(buildpack BuildModule) {
	b.dependencies.AddModules(buildpack)
}

func (b *PackageBuilder) AddDependencies(main BuildModule, dependencies []BuildModule) {
	b.dependencies.AddModules(main, dependencies...)
}

func (b *PackageBuilder) MustBeFlatten(module BuildModule) bool {
	return b.flattenAllBuildpacks || (b.dependencies.IsFlatten(module))
}

func (b *PackageBuilder) FlattenModules() [][]BuildModule {
	return b.dependencies.GetFlattenModules()
}

func (b *PackageBuilder) finalizeImage(image WorkableImage, tmpDir string) error {
	if err := dist.SetLabel(image, MetadataLabel, &Metadata{
		ModuleInfo: b.buildpack.Descriptor().Info(),
		Stacks:     b.resolvedStacks(),
	}); err != nil {
		return err
	}

	collectionToAdd := map[string]toAdd{}
	// Let's create the tarball for each module
	for _, bp := range append(b.dependencies.Modules(), b.buildpack) {
		bpLayerTar, err := ToLayerTar(tmpDir, bp)
		if err != nil {
			return err
		}

		diffID, err := dist.LayerDiffID(bpLayerTar)
		if err != nil {
			return errors.Wrapf(err,
				"getting content hashes for buildpack %s",
				style.Symbol(bp.Descriptor().Info().FullName()),
			)
		}
		collectionToAdd[bp.Descriptor().Info().FullName()] = toAdd{
			tarPath: bpLayerTar,
			diffID:  diffID.String(),
			module:  bp,
		}
	}

	if b.flattenAllBuildpacks {
		// let's squash all buildpacks in a single layer
		modFlattenTmpDir := filepath.Join(tmpDir, "buildpack-all-flatten")
		if err := os.MkdirAll(modFlattenTmpDir, os.ModePerm); err != nil {
			return errors.Wrap(err, "creating flatten temp dir")
		}
		finalTarPath := filepath.Join(modFlattenTmpDir, "all-flatten.tar")

		var tarsPath []string
		for key := range collectionToAdd {
			if !b.skipFlatten(key) {
				m := collectionToAdd[key]
				tarsPath = append(tarsPath, m.tarPath)
			}
		}

		err := archive.MergeTars(finalTarPath, tarsPath...)
		if err != nil {
			return errors.Wrap(err, "merging modules tar files")
		}

		diffID, err := dist.LayerDiffID(finalTarPath)
		if err != nil {
			return errors.Wrapf(err, "adding layer %s", finalTarPath)
		}

		// Update the diffId and tar path for each module squashed
		for key := range collectionToAdd {
			if !b.skipFlatten(key) {
				addModule := collectionToAdd[key]
				addModule.tarPath = finalTarPath
				addModule.diffID = diffID.String()
				collectionToAdd[key] = addModule
			}
		}
	} else {
		// Let's squash build modules
		for i, flattenModules := range b.FlattenModules() {
			modFlattenTmpDir := filepath.Join(tmpDir, fmt.Sprintf("%s-flatten-%s", "buildpack", strconv.Itoa(i)))
			if err := os.MkdirAll(modFlattenTmpDir, os.ModePerm); err != nil {
				return errors.Wrap(err, "creating flatten temp dir")
			}
			flattenTarFilePath := filepath.Join(modFlattenTmpDir, fmt.Sprintf("%s-flatten-%s.tar", "buildpack", strconv.Itoa(i)))

			var tarsPath []string
			for _, module := range flattenModules {
				if !b.skipFlatten(module.Descriptor().Info().FullName()) {
					m := collectionToAdd[module.Descriptor().Info().FullName()]
					tarsPath = append(tarsPath, m.tarPath)
				}
			}

			err := archive.MergeTars(flattenTarFilePath, tarsPath...)
			if err != nil {
				return errors.Wrap(err, "merging modules tar files")
			}

			diffID, err := dist.LayerDiffID(flattenTarFilePath)
			if err != nil {
				return errors.Wrapf(err, "adding layer %s", flattenTarFilePath)
			}

			// Update the diffId and tar path for each module squashed
			for _, module := range flattenModules {
				if !b.skipFlatten(module.Descriptor().Info().FullName()) {
					addModule := collectionToAdd[module.Descriptor().Info().FullName()]
					addModule.tarPath = flattenTarFilePath
					addModule.diffID = diffID.String()
					collectionToAdd[module.Descriptor().Info().FullName()] = addModule
				}
			}
		}
	}

	bpLayers := dist.ModuleLayers{}
	diffIDAdded := map[string]string{}

	for key := range collectionToAdd {
		module := collectionToAdd[key]
		bp := module.module
		addLayer := true
		if b.MustBeFlatten(bp) {
			if _, ok := diffIDAdded[module.diffID]; !ok {
				diffIDAdded[module.diffID] = module.tarPath
			} else {
				addLayer = false
			}
		}
		if addLayer {
			if err := image.AddLayerWithDiffID(module.tarPath, module.diffID); err != nil {
				return errors.Wrapf(err, "adding layer tar for buildpack %s", style.Symbol(bp.Descriptor().Info().FullName()))
			}
		}

		dist.AddToLayersMD(bpLayers, bp.Descriptor(), module.diffID)
	}

	if err := dist.SetLabel(image, dist.BuildpackLayersLabel, bpLayers); err != nil {
		return err
	}

	return nil
}

func (b *PackageBuilder) finalizeExtensionImage(image WorkableImage, tmpDir string) error {
	if err := dist.SetLabel(image, MetadataLabel, &Metadata{
		ModuleInfo: b.extension.Descriptor().Info(),
	}); err != nil {
		return err
	}

	exLayers := dist.ModuleLayers{}
	exLayerTar, err := ToLayerTar(tmpDir, b.extension)
	if err != nil {
		return err
	}

	diffID, err := dist.LayerDiffID(exLayerTar)
	if err != nil {
		return errors.Wrapf(err,
			"getting content hashes for extension %s",
			style.Symbol(b.extension.Descriptor().Info().FullName()),
		)
	}

	if err := image.AddLayerWithDiffID(exLayerTar, diffID.String()); err != nil {
		return errors.Wrapf(err, "adding layer tar for extension %s", style.Symbol(b.extension.Descriptor().Info().FullName()))
	}

	dist.AddToLayersMD(exLayers, b.extension.Descriptor(), diffID.String())

	if err := dist.SetLabel(image, dist.ExtensionLayersLabel, exLayers); err != nil {
		return err
	}

	return nil
}

func (b *PackageBuilder) validate() error {
	if b.buildpack == nil && b.extension == nil {
		return errors.New("buildpack or extension must be set")
	}

	// we don't need to validate extensions because there are no order or stacks in extensions
	if b.buildpack != nil && b.extension == nil {
		if err := validateBuildpacks(b.buildpack, b.dependencies.Modules()); err != nil {
			return err
		}

		if len(b.resolvedStacks()) == 0 {
			return errors.Errorf("no compatible stacks among provided buildpacks")
		}
	}

	return nil
}

func (b *PackageBuilder) resolvedStacks() []dist.Stack {
	stacks := b.buildpack.Descriptor().Stacks()
	for _, bp := range b.dependencies.Modules() {
		bpd := bp.Descriptor()

		if len(stacks) == 0 {
			stacks = bpd.Stacks()
		} else if len(bpd.Stacks()) > 0 { // skip over "meta-buildpacks"
			stacks = stack.MergeCompatible(stacks, bpd.Stacks())
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

	tempDirName := ""
	if b.buildpack != nil {
		tempDirName = "package-buildpack"
	} else if b.extension != nil {
		tempDirName = "extension-buildpack"
	}

	tmpDir, err := os.MkdirTemp("", tempDirName)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if b.buildpack != nil {
		if err := b.finalizeImage(layoutImage, tmpDir); err != nil {
			return err
		}
	} else if b.extension != nil {
		if err := b.finalizeExtensionImage(layoutImage, tmpDir); err != nil {
			return err
		}
	}
	layoutDir, err := os.MkdirTemp(tmpDir, "oci-layout")
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

func (b *PackageBuilder) skipFlatten(bpFullName string) bool {
	for _, excludeBP := range b.flattenExcludeBuildpacks {
		if excludeBP == bpFullName {
			return true
		}
	}
	return false
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
	tempDirName := ""
	if b.buildpack != nil {
		tempDirName = "package-buildpack"
	} else if b.extension != nil {
		tempDirName = "extension-buildpack"
	}

	tmpDir, err := os.MkdirTemp("", tempDirName)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	if b.buildpack != nil {
		if err := b.finalizeImage(image, tmpDir); err != nil {
			return nil, err
		}
	} else if b.extension != nil {
		if err := b.finalizeExtensionImage(image, tmpDir); err != nil {
			return nil, err
		}
	}

	if err := image.Save(); err != nil {
		return nil, err
	}

	return image, nil
}

func validateBuildpacks(mainBP BuildModule, depBPs []BuildModule) error {
	depsWithRefs := map[string][]dist.ModuleInfo{}

	for _, bp := range depBPs {
		depsWithRefs[bp.Descriptor().Info().FullName()] = nil
	}

	for _, bp := range append([]BuildModule{mainBP}, depBPs...) { // List of everything
		bpd := bp.Descriptor()
		for _, orderEntry := range bpd.Order() {
			for _, groupEntry := range orderEntry.Group {
				bpFullName, err := groupEntry.ModuleInfo.FullNameWithVersion()
				if err != nil {
					return errors.Wrapf(
						err,
						"buildpack %s must specify a version when referencing buildpack %s",
						style.Symbol(bpd.Info().FullName()),
						style.Symbol(bpFullName),
					)
				}
				if _, ok := depsWithRefs[bpFullName]; !ok {
					return errors.Errorf(
						"buildpack %s references buildpack %s which is not present",
						style.Symbol(bpd.Info().FullName()),
						style.Symbol(bpFullName),
					)
				}

				depsWithRefs[bpFullName] = append(depsWithRefs[bpFullName], bpd.Info())
			}
		}
	}

	for bp, refs := range depsWithRefs {
		if len(refs) == 0 {
			return errors.Errorf(
				"buildpack %s is not used by buildpack %s",
				style.Symbol(bp),
				style.Symbol(mainBP.Descriptor().Info().FullName()),
			)
		}
	}

	return nil
}
