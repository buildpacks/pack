package buildpack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/index"
	"github.com/buildpacks/imgutil/layer"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/pkg/logging"

	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/internal/stringset"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/dist"
	pkgImg "github.com/buildpacks/pack/pkg/image"
)

type ImageFactory interface {
	NewImage(repoName string, local bool, platform v1.Platform) (imgutil.Image, error)
}

type IndexFactory interface {
	// load ManifestList from local storage with the given name
	LoadIndex(reponame string, opts ...index.Option) (imgutil.ImageIndex, error)
}

type WorkableImage interface {
	// Getters
	OS() (string, error)
	Architecture() (string, error)
	Variant() (string, error)
	OSVersion() (string, error)
	Features() ([]string, error)
	OSFeatures() ([]string, error)
	URLs() ([]string, error)
	Annotations() (map[string]string, error)

	// Setters
	SetLabel(string, string) error
	SetOS(string) error
	SetArchitecture(string) error
	SetVariant(string) error
	SetOSVersion(string) error
	SetFeatures([]string) error
	SetOSFeatures([]string) error
	SetURLs([]string) error
	SetAnnotations(map[string]string) error
	Save(...string) error

	AddLayerWithDiffID(path, diffID string) error

	// Misc

	Digest() (v1.Hash, error)
	MediaType() (types.MediaType, error)
	ManifestSize() (int64, error)
}

type layoutImage struct {
	v1.Image
	os, arch, variant, osVersion string
	features, osFeatures, urls   []string
	annotations                  map[string]string
}

var _ WorkableImage = (*layoutImage)(nil)
var _ imgutil.EditableImage = (*layoutImage)(nil)

type toAdd struct {
	tarPath string
	diffID  string
	module  BuildModule
}

func (i *layoutImage) OS() (os string, err error) {
	if i.os != "" {
		return i.os, nil
	}

	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return os, err
	}
	if cfg == nil {
		return os, imgutil.ErrConfigFileUndefined
	}

	if cfg.OS != "" {
		return cfg.OS, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return os, err
	}

	format, err := i.MediaType()
	if err != nil {
		return os, err
	}

	return os, imgutil.ErrOSUndefined(format, digest.String())
}

func (i *layoutImage) Architecture() (arch string, err error) {
	if i.arch != "" {
		return i.arch, nil
	}

	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return arch, err
	}
	if cfg == nil {
		return arch, imgutil.ErrConfigFileUndefined
	}

	if cfg.Architecture != "" {
		return cfg.Architecture, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return arch, err
	}

	format, err := i.MediaType()
	if err != nil {
		return arch, err
	}

	return arch, imgutil.ErrArchUndefined(format, digest.String())
}

func (i *layoutImage) Variant() (variant string, err error) {
	if i.variant != "" {
		return i.variant, nil
	}

	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return variant, err
	}
	if cfg == nil {
		return variant, imgutil.ErrConfigFileUndefined
	}

	if cfg.Variant != "" {
		return cfg.Variant, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return variant, err
	}

	format, err := i.MediaType()
	if err != nil {
		return variant, err
	}

	return variant, imgutil.ErrVariantUndefined(format, digest.String())
}

func (i *layoutImage) OSVersion() (osVersion string, err error) {
	if i.osVersion != "" {
		return i.osVersion, nil
	}

	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return osVersion, err
	}
	if cfg == nil {
		return osVersion, imgutil.ErrConfigFileUndefined
	}

	if cfg.OSVersion != "" {
		return cfg.OSVersion, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return osVersion, err
	}

	format, err := i.MediaType()
	if err != nil {
		return osVersion, err
	}

	return osVersion, imgutil.ErrOSVersionUndefined(format, digest.String())
}

func (i *layoutImage) Features() (features []string, err error) {
	if len(i.features) != 0 {
		return i.features, nil
	}

	mfest, err := i.Image.Manifest()
	if err != nil {
		return features, err
	}
	if mfest == nil {
		return features, imgutil.ErrManifestUndefined
	}

	if mfest.Subject == nil {
		return features, imgutil.ErrManifestUndefined
	}

	if mfest.Subject.Platform == nil {
		return features, imgutil.ErrPlatformUndefined
	}

	if features = mfest.Subject.Platform.Features; len(features) != 0 {
		return features, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return features, err
	}

	format, err := i.MediaType()
	if err != nil {
		return features, err
	}

	return features, imgutil.ErrFeaturesUndefined(format, digest.String())
}

func (i *layoutImage) OSFeatures() (osFeatures []string, err error) {
	if len(i.osFeatures) != 0 {
		return i.osFeatures, nil
	}

	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return osFeatures, err
	}
	if cfg == nil {
		return osFeatures, imgutil.ErrConfigFileUndefined
	}

	if len(cfg.OSFeatures) != 0 {
		return cfg.OSFeatures, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return osFeatures, err
	}

	format, err := i.MediaType()
	if err != nil {
		return osFeatures, err
	}

	return osFeatures, imgutil.ErrOSVersionUndefined(format, digest.String())
}

func (i *layoutImage) URLs() (urls []string, err error) {
	if len(i.urls) != 0 {
		return i.urls, nil
	}

	mfest, err := i.Image.Manifest()
	if err != nil {
		return urls, err
	}
	if mfest == nil {
		return urls, imgutil.ErrManifestUndefined
	}

	if urls = mfest.Config.URLs; len(urls) != 0 {
		return urls, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return urls, err
	}

	format, err := i.MediaType()
	if err != nil {
		return urls, err
	}

	return urls, imgutil.ErrURLsUndefined(format, digest.String())
}

func (i *layoutImage) Annotations() (annos map[string]string, err error) {
	if len(i.annotations) != 0 {
		return i.annotations, nil
	}

	mfest, err := i.Image.Manifest()
	if err != nil {
		return annos, err
	}
	if mfest == nil {
		return annos, imgutil.ErrManifestUndefined
	}

	if annos = mfest.Annotations; len(annos) != 0 {
		return annos, nil
	}

	digest, err := i.Digest()
	if err != nil {
		return annos, err
	}

	format, err := i.MediaType()
	if err != nil {
		return annos, err
	}

	return annos, imgutil.ErrAnnotationsUndefined(format, digest.String())
}

func (i *layoutImage) SetOS(os string) error {
	i.os = os
	return nil
}

func (i *layoutImage) SetArchitecture(arch string) error {
	i.arch = arch
	return nil
}

func (i *layoutImage) SetVariant(variant string) error {
	i.variant = variant
	return nil
}

func (i *layoutImage) SetOSVersion(osVersion string) error {
	i.osVersion = osVersion
	return nil
}

func (i *layoutImage) SetFeatures(features []string) error {
	if len(features) == 0 {
		features = make([]string, 0)
	}

	if len(i.features) == 0 {
		i.features = make([]string, 0)
	}

	i.features = append(i.features, features...)
	return nil
}

func (i *layoutImage) SetOSFeatures(osFeatures []string) error {
	if len(osFeatures) == 0 {
		osFeatures = make([]string, 0)
	}

	if len(i.osFeatures) == 0 {
		i.osFeatures = make([]string, 0)
	}

	i.osFeatures = append(i.osFeatures, osFeatures...)
	return nil
}

func (i *layoutImage) SetURLs(urls []string) error {
	if len(urls) == 0 {
		urls = make([]string, 0)
	}

	if len(i.urls) == 0 {
		i.urls = make([]string, 0)
	}

	i.urls = append(i.urls, urls...)
	return nil
}

func (i *layoutImage) SetAnnotations(annos map[string]string) error {
	if len(annos) == 0 {
		annos = make(map[string]string, 0)
	}

	if len(i.annotations) == 0 {
		i.annotations = make(map[string]string, 0)
	}

	for k, v := range annos {
		i.annotations[k] = v
	}
	return nil
}

func (i *layoutImage) ManifestSize() (int64, error) {
	return i.Image.Size()
}

func (i *layoutImage) Save(_ ...string) error {
	config, err := i.ConfigFile()
	if err != nil {
		return err
	}
	if config == nil {
		return imgutil.ErrConfigFileUndefined
	}

	mfest, err := i.Manifest()
	if err != nil {
		return err
	}
	if mfest == nil {
		return imgutil.ErrManifestUndefined
	}

	digest, err := i.Digest()
	if err != nil {
		return err
	}

	mutateImage := false
	cfg := config.DeepCopy()
	desc := mfest.Config.DeepCopy()
	desc.Size, _ = partial.Size(i)
	desc.MediaType = mfest.MediaType
	desc.Digest = digest
	if desc.Platform == nil {
		desc.Platform = &v1.Platform{}
	}

	if i.os != "" && i.os != config.OS {
		mutateImage = true
		cfg.OS = i.os
		desc.Platform.OS = i.os
	}

	if i.arch != "" && i.arch != config.Architecture {
		mutateImage = true
		cfg.Architecture = i.arch
		desc.Platform.Architecture = i.arch
	}

	if i.variant != "" && i.variant != config.Variant {
		mutateImage = true
		cfg.Variant = i.variant
		desc.Platform.Variant = i.variant
	}

	if i.osVersion != "" && i.osVersion != config.OSVersion {
		mutateImage = true
		cfg.OSVersion = i.osVersion
		desc.Platform.OSVersion = i.osVersion
	}

	if len(i.features) != 0 && !slices.Equal(i.features, desc.Platform.Features) {
		mutateImage = true
		desc.Platform.Features = append(desc.Platform.Features, i.features...)
	}

	if len(i.osFeatures) != 0 && !slices.Equal(i.osFeatures, config.OSFeatures) {
		mutateImage = true
		cfg.OSFeatures = append(cfg.OSFeatures, i.osFeatures...)
		desc.Platform.OSFeatures = cfg.OSFeatures
	}

	if len(i.urls) != 0 && !slices.Equal(i.urls, desc.URLs) {
		mutateImage = true
		desc.URLs = append(desc.URLs, i.urls...)
	}

	if len(i.annotations) != 0 && !mapContains(i.annotations, mfest.Annotations) {
		mutateImage = true
		for k, v := range i.annotations {
			desc.Annotations[k] = v
		}
		i.Image = mutate.Annotations(i, desc.Annotations).(v1.Image)
	}

	if mutateImage {
		i.Image, err = mutate.ConfigFile(i.Image, cfg)
		i.Image = mutate.Subject(i, *desc).(v1.Image)
	}
	return err
}

func (i *layoutImage) SetLabel(key string, val string) error {
	configFile, err := i.ConfigFile()
	if err != nil {
		return err
	}
	if configFile == nil {
		return imgutil.ErrConfigFileUndefined
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
	exclude []string
	logger  logging.Logger
	factory archive.TarWriterFactory
}

type PackageBuilder struct {
	buildpack                BuildModule
	extension                BuildModule
	logger                   logging.Logger
	layerWriterFactory       archive.TarWriterFactory
	dependencies             ManagedCollection
	imageFactory             ImageFactory
	indexFactory             IndexFactory
	flattenAllBuildpacks     bool
	flattenExcludeBuildpacks []string
}

// TODO: Rename to PackageBuilder
func NewBuilder(imageFactory ImageFactory, indexFactory IndexFactory, ops ...PackageBuilderOption) *PackageBuilder {
	opts := &options{}
	for _, op := range ops {
		if err := op(opts); err != nil {
			return nil
		}
	}
	moduleManager := NewManagedCollectionV1(opts.flatten)
	return &PackageBuilder{
		imageFactory:             imageFactory,
		indexFactory:             indexFactory,
		dependencies:             moduleManager,
		flattenAllBuildpacks:     opts.flatten,
		flattenExcludeBuildpacks: opts.exclude,
		logger:                   opts.logger,
		layerWriterFactory:       opts.factory,
	}
}

func FlattenAll() PackageBuilderOption {
	return func(o *options) error {
		o.flatten = true
		return nil
	}
}

func DoNotFlatten(exclude []string) PackageBuilderOption {
	return func(o *options) error {
		o.flatten = true
		o.exclude = exclude
		return nil
	}
}

func WithLogger(logger logging.Logger) PackageBuilderOption {
	return func(o *options) error {
		o.logger = logger
		return nil
	}
}

func WithLayerWriterFactory(factory archive.TarWriterFactory) PackageBuilderOption {
	return func(o *options) error {
		o.factory = factory
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

func (b *PackageBuilder) ShouldFlatten(module BuildModule) bool {
	return b.flattenAllBuildpacks || (b.dependencies.ShouldFlatten(module))
}

func (b *PackageBuilder) FlattenedModules() [][]BuildModule {
	return b.dependencies.FlattenedModules()
}

func (b *PackageBuilder) AllModules() []BuildModule {
	all := b.dependencies.ExplodedModules()
	for _, modules := range b.dependencies.FlattenedModules() {
		all = append(all, modules...)
	}
	return all
}

func (b *PackageBuilder) finalizeImage(image WorkableImage, tmpDir string) error {
	if err := dist.SetLabel(image, MetadataLabel, &Metadata{
		ModuleInfo: b.buildpack.Descriptor().Info(),
		Stacks:     b.resolvedStacks(),
	}); err != nil {
		return err
	}

	collectionToAdd := map[string]toAdd{}
	var individualBuildModules []BuildModule

	// Let's create the tarball for each flatten module
	if len(b.FlattenedModules()) > 0 {
		buildModuleWriter := NewBuildModuleWriter(b.logger, b.layerWriterFactory)
		excludedModules := Set(b.flattenExcludeBuildpacks)

		var (
			finalTarPath string
			err          error
		)
		for i, additionalModules := range b.FlattenedModules() {
			modFlattenTmpDir := filepath.Join(tmpDir, fmt.Sprintf("buildpack-%s-flatten", strconv.Itoa(i)))
			if err := os.MkdirAll(modFlattenTmpDir, os.ModePerm); err != nil {
				return errors.Wrap(err, "creating flatten temp dir")
			}

			if b.flattenAllBuildpacks {
				// include the buildpack itself
				additionalModules = append(additionalModules, b.buildpack)
			}
			finalTarPath, individualBuildModules, err = buildModuleWriter.NToLayerTar(modFlattenTmpDir, fmt.Sprintf("buildpack-flatten-%s", strconv.Itoa(i)), additionalModules, excludedModules)
			if err != nil {
				return errors.Wrapf(err, "adding layer %s", finalTarPath)
			}

			diffID, err := dist.LayerDiffID(finalTarPath)
			if err != nil {
				return errors.Wrapf(err, "calculating diffID for layer %s", finalTarPath)
			}

			for _, module := range additionalModules {
				collectionToAdd[module.Descriptor().Info().FullName()] = toAdd{
					tarPath: finalTarPath,
					diffID:  diffID.String(),
					module:  module,
				}
			}
		}
	}

	if !b.flattenAllBuildpacks || len(b.FlattenedModules()) == 0 {
		individualBuildModules = append(individualBuildModules, b.buildpack)
	}

	// Let's create the tarball for each individual module
	for _, bp := range append(b.dependencies.ExplodedModules(), individualBuildModules...) {
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

	bpLayers := dist.ModuleLayers{}
	diffIDAdded := map[string]string{}

	for key := range collectionToAdd {
		module := collectionToAdd[key]
		bp := module.module
		addLayer := true
		if b.ShouldFlatten(bp) {
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

	return dist.SetLabel(image, dist.BuildpackLayersLabel, bpLayers)
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
		if err := validateBuildpacks(b.buildpack, b.AllModules()); err != nil {
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
	for _, bp := range b.AllModules() {
		bpd := bp.Descriptor()

		if len(stacks) == 0 {
			stacks = bpd.Stacks()
		} else if len(bpd.Stacks()) > 0 { // skip over "meta-buildpacks"
			stacks = stack.MergeCompatible(stacks, bpd.Stacks())
		}
	}

	return stacks
}

func (b *PackageBuilder) SaveAsFile(path, version string, target dist.Target, idx imgutil.ImageIndex, labels map[string]string) error {
	if err := b.validate(); err != nil {
		return err
	}

	platform := target.Platform()
	platform.OSVersion = version
	layoutImage, err := newLayoutImage(*platform)
	if err != nil {
		return errors.Wrap(err, "creating layout image")
	}

	for labelKey, labelValue := range labels {
		if err = layoutImage.SetLabel(labelKey, labelValue); err != nil {
			return errors.Wrapf(err, "adding label %s=%s", labelKey, labelValue)
		}
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

	if err := updateLayoutImagePlatform(layoutImage, target); err != nil {
		return err
	}

	if err := layoutImage.Save(); err != nil {
		return err
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

	_, digest, err := getImageDigest(path, layoutImage)
	if err != nil {
		return err
	}

	if idx != nil {
		if err := idx.Add(digest, imgutil.WithLocalImage(layoutImage)); err != nil {
			return err
		}

		if err = idx.Save(); err != nil {
			return err
		}
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

func (b *PackageBuilder) SaveAsMultiArchFile(path, version string, targets []dist.Target, idx imgutil.ImageIndex, labels map[string]string) error {
	if err := b.validate(); err != nil {
		return err
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

	layoutDir, err := os.MkdirTemp(tmpDir, "oci-layout")
	if err != nil {
		return errors.Wrap(err, "creating oci-layout temp dir")
	}

	p, err := layout.Write(layoutDir, empty.Index)
	if err != nil {
		return errors.Wrap(err, "writing index")
	}

	for _, target := range targets {
		if err := target.Range(func(target dist.Target, distroName, distroVersion string) error {
			target.Distributions = []dist.Distribution{{Name: distroName, Versions: []string{distroVersion}}}
			platform := target.Platform()
			platform.OSVersion = version
			layoutImage, err := newLayoutImage(*platform)
			if err != nil {
				return errors.Wrap(err, "creating layout image")
			}

			for labelKey, labelValue := range labels {
				if err = layoutImage.SetLabel(labelKey, labelValue); err != nil {
					return errors.Wrapf(err, "adding label %s=%s", labelKey, labelValue)
				}
			}

			if b.buildpack != nil {
				if err := b.finalizeImage(layoutImage, tmpDir); err != nil {
					return err
				}
			} else if b.extension != nil {
				if err := b.finalizeExtensionImage(layoutImage, tmpDir); err != nil {
					return err
				}
			}

			if err := updateLayoutImagePlatform(layoutImage, target); err != nil {
				return err
			}

			if err := layoutImage.Save(); err != nil {
				return err
			}

			if err := p.AppendImage(layoutImage); err != nil {
				return errors.Wrap(err, "writing layout")
			}

			_, digest, err := getImageDigest(path, layoutImage)
			if err != nil {
				return err
			}

			if idx == nil {
				return nil
			}

			if err := idx.Add(digest, imgutil.WithLocalImage(layoutImage)); err != nil {
				return err
			}

			return idx.Save()
		}); err != nil {
			return err
		}
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

func newLayoutImage(platform v1.Platform) (*layoutImage, error) {
	i := empty.Image

	configFile, err := i.ConfigFile()
	if err != nil {
		return nil, err
	}

	configFile.OS = platform.OS
	configFile.Architecture = platform.Architecture
	configFile.Variant = platform.Variant
	configFile.OSVersion = platform.OSVersion
	configFile.OSFeatures = platform.OSFeatures
	i, err = mutate.ConfigFile(i, configFile)
	if err != nil {
		return nil, err
	}

	if platform.OS == "windows" {
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

func (b *PackageBuilder) SaveAsImage(repoName, version string, publish bool, target dist.Target, idx imgutil.ImageIndex, labels map[string]string) (imgutil.Image, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	platform := *target.Platform()
	imageName := repoName
	if idx != nil {
		imageName += ":" + strings.Join(strings.Split(strings.ReplaceAll(PlatformSafeName("", target), "@", "-"), "-")[1:], "-")
	}

	image, err := b.imageFactory.NewImage(imageName, !publish, platform)
	if err != nil {
		return nil, errors.Wrapf(err, "creating image")
	}

	for labelKey, labelValue := range labels {
		if err = image.SetLabel(labelKey, labelValue); err != nil {
			return nil, errors.Wrapf(err, "adding label %s=%s", labelKey, labelValue)
		}
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

	underlyingImage := image.UnderlyingImage()
	ref, digest, err := getImageDigest(repoName, underlyingImage)
	if err != nil {
		return nil, err
	}

	// handle Local Image Digest
	if underlyingImage == nil {
		id, err := image.Identifier()
		if err != nil {
			return nil, err
		}

		digest = ref.Context().Digest("sha256:" + id.String())
	}

	if err := updateImagePlatform(image, target); err != nil {
		return nil, err
	}

	features, _ := image.Features()
	osFeatures, _ := image.OSFeatures()
	urls, _ := image.URLs()
	annotations, _ := image.Annotations()

	var featuresFound, osFeaturesFound, urlsFound, annosFound bool
	featuresFound = sliceContains(features, target.Specs.Features)
	osFeaturesFound = sliceContains(osFeatures, target.Specs.OSFeatures)
	urlsFound = sliceContains(urls, target.Specs.URLs)
	annosFound = mapContains(annotations, target.Specs.Annotations)

	// getAddtionalImageNames(ref, target)...
	if err := image.Save(); err != nil {
		return nil, err
	}

	if idx == nil {
		return image, nil
	}

	if err := idx.Add(digest, imgutil.WithLocalImage(image)); err != nil {
		return nil, err
	}

	if !featuresFound {
		if err := idx.SetFeatures(digest, features); err != nil {
			return nil, err
		}
	}

	if !osFeaturesFound {
		if err := idx.SetOSFeatures(digest, osFeatures); err != nil {
			return nil, err
		}
	}

	if !urlsFound {
		if err := idx.SetURLs(digest, urls); err != nil {
			return nil, err
		}
	}

	if !annosFound {
		annos, err := target.Annotations()
		if err != nil {
			return nil, err
		}

		if len(annotations) == 0 {
			annotations = make(map[string]string)
		}

		for k, v := range annos {
			annotations[k] = v
		}

		if err := idx.SetAnnotations(digest, annotations); err != nil {
			return nil, err
		}
	}

	return image, idx.Save()
}

func mapContains(src, contains map[string]string) bool {
	for k, v := range contains {
		if srcValue, ok := src[k]; !ok || srcValue != v {
			return false
		}
	}
	return true
}

func sliceContains(src, contains []string) bool {
	_, missing, _ := stringset.Compare(contains, src)
	return len(missing) == 0
}

func updateLayoutImagePlatform(image *layoutImage, target dist.Target) (err error) {
	var (
		config *v1.ConfigFile
		mfest  *v1.Manifest
	)

	if config, err = image.ConfigFile(); err != nil {
		return err
	}
	if config == nil {
		return imgutil.ErrConfigFileUndefined
	}

	if mfest, err = image.Manifest(); err != nil {
		return err
	}
	if mfest == nil {
		return imgutil.ErrManifestUndefined
	}

	platform := target.Platform()
	if mfest.Config.Platform == nil {
		mfest.Config.Platform = &v1.Platform{}
	}

	if err := updatePlatformPrimitives(image, platform, config, mfest); err != nil {
		return err
	}

	return updatePlatformSlicesAndMaps(image, target, config, mfest)
}

func updatePlatformPrimitives(image imgutil.EditableImage, platform *v1.Platform, config *v1.ConfigFile, mfest *v1.Manifest) error {
	if platform.OS != "" && (config.OS != platform.OS || mfest.Config.Platform.OS != platform.OS) {
		if err := image.SetOS(platform.OS); err != nil {
			return err
		}
	}

	if platform.Architecture != "" && (config.Architecture != platform.Architecture || mfest.Config.Platform.Architecture != platform.Architecture) {
		if err := image.SetArchitecture(platform.Architecture); err != nil {
			return err
		}
	}

	if platform.Variant != "" && (config.Variant != platform.Variant || mfest.Config.Platform.Variant != platform.Variant) {
		if err := image.SetVariant(platform.Variant); err != nil {
			return err
		}
	}

	if platform.OSVersion != "" && (config.OSVersion != platform.OSVersion || mfest.Config.Platform.OSVersion != platform.OSVersion) {
		if err := image.SetOSVersion(platform.OSVersion); err != nil {
			return err
		}
	}

	return nil
}

func updatePlatformSlicesAndMaps(image imgutil.EditableImage, target dist.Target, config *v1.ConfigFile, mfest *v1.Manifest) error {
	platform := target.Platform()
	if len(target.Specs.Features) > 0 && !slices.Equal(mfest.Config.Platform.Features, platform.Features) {
		if err := image.SetFeatures(target.Specs.Features); err != nil {
			return err
		}
	}

	if len(target.Specs.OSFeatures) > 0 && !(slices.Equal(mfest.Config.Platform.OSFeatures, platform.OSFeatures) || slices.Equal(config.OSFeatures, platform.OSFeatures)) {
		if err := image.SetOSFeatures(target.Specs.OSFeatures); err != nil {
			return err
		}
	}

	if len(target.Specs.URLs) > 0 && !slices.Equal(mfest.Config.URLs, target.Specs.URLs) {
		if err := image.SetURLs(target.Specs.URLs); err != nil {
			return err
		}
	}

	if len(target.Specs.Annotations) > 0 && !mapContains(mfest.Annotations, target.Specs.Annotations) {
		if err := image.SetAnnotations(target.Specs.Annotations); err != nil {
			return err
		}
	}

	return nil
}

func updateImagePlatform(image imgutil.Image, target dist.Target) (err error) {
	var config *v1.ConfigFile
	var mfest *v1.Manifest

	platform := target.Platform()
	underlyingImage := image.UnderlyingImage()
	if underlyingImage == nil {
		return updatePlatformPrimitives(image, platform, &v1.ConfigFile{}, &v1.Manifest{})
	}

	if config, err = underlyingImage.ConfigFile(); err != nil {
		return err
	}
	if config == nil {
		return imgutil.ErrConfigFileUndefined
	}

	if mfest, err = image.UnderlyingImage().Manifest(); err != nil {
		return err
	}
	if mfest == nil {
		return imgutil.ErrManifestUndefined
	}

	if mfest.Config.Platform == nil {
		mfest.Config.Platform = &v1.Platform{}
	}

	if err := updatePlatformPrimitives(image, platform, config, mfest); err != nil {
		return err
	}

	if image.Kind() != pkgImg.LOCAL {
		return updatePlatformSlicesAndMaps(image, target, config, mfest)
	}

	return nil
}

func getImageDigest(repoName string, image v1.Image) (ref name.Reference, digest name.Digest, err error) {
	ref, err = name.ParseReference(repoName)
	if err != nil {
		return ref, digest, err
	}

	// for local.Image imgutil#UnderlyingImage is nil
	if image == nil {
		return ref, digest, nil
	}

	hash, err := image.Digest()
	if err != nil {
		return ref, digest, err
	}

	return ref, ref.Context().Digest(hash.String()), nil
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
