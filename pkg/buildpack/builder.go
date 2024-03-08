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
	"github.com/buildpacks/imgutil/index"
	"github.com/buildpacks/imgutil/layer"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/pkg/logging"

	"github.com/buildpacks/pack/internal/stack"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/dist"
	pkgImg "github.com/buildpacks/pack/pkg/image"
)

type ImageFactory interface {
	NewImage(repoName string, local bool, target dist.Target) (imgutil.Image, error)
}

type IndexFactory interface {
	// load ManifestList from local storage with the given name
	LoadIndex(reponame string, opts ...index.Option) (imgutil.ImageIndex, error)
}

type WorkableImage interface {
	SetLabel(string, string) error
	AddLayerWithDiffID(path, diffID string) error
	SetOS(string) error
	SetArchitecture(string) error
	SetVariant(string) error
	SetOSVersion(string) error
	SetFeatures([]string) error
	SetOSFeatures([]string) error
	SetURLs([]string) error
	SetAnnotations(map[string]string) error
	Save(...string) error
}

type layoutImage struct {
	v1.Image
	os, arch, variant, osVersion string
	features, osFeatures, urls   []string
	annotations                  map[string]string
}

var _ WorkableImage = (*layoutImage)(nil)

type toAdd struct {
	tarPath string
	diffID  string
	module  BuildModule
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
	i.features = append(i.features, features...)
	return nil
}

func (i *layoutImage) SetOSFeatures(osFeatures []string) error {
	i.osFeatures = append(i.osFeatures, osFeatures...)
	return nil
}

func (i *layoutImage) SetURLs(urls []string) error {
	i.urls = append(i.urls, urls...)
	return nil
}

func (i *layoutImage) SetAnnotations(annos map[string]string) error {
	for k, v := range annos {
		i.annotations[k] = v
	}
	return nil
}

func (i *layoutImage) Save(...string) error {
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

	cfg := config.DeepCopy()
	desc := mfest.Config.DeepCopy()
	desc.Size, _ = partial.Size(i)
	desc.MediaType = mfest.MediaType
	desc.Digest = digest
	if desc.Platform == nil {
		desc.Platform = &v1.Platform{}
	}

	if i.os != "" {
		cfg.OS = i.os
		desc.Platform.OS = i.os
	}

	if i.arch != "" {
		cfg.Architecture = i.arch
		desc.Platform.Architecture = i.arch
	}

	if i.variant != "" {
		cfg.Variant = i.variant
		desc.Platform.Variant = i.variant
	}

	if i.osVersion != "" {
		cfg.OSVersion = i.osVersion
		desc.Platform.OSVersion = i.osVersion
	}

	if len(i.features) != 0 {
		desc.Platform.Features = append(desc.Platform.Features, i.features...)
	}

	if len(i.osFeatures) != 0 {
		cfg.OSFeatures = append(cfg.OSFeatures, i.osFeatures...)
		desc.Platform.OSFeatures = cfg.OSFeatures
	}

	if len(i.urls) != 0 {
		desc.URLs = append(desc.URLs, i.urls...)
	}

	if len(i.annotations) != 0 {
		for k, v := range i.annotations {
			desc.Annotations[k] = v
		}
		i.Image = mutate.Annotations(i, desc.Annotations).(v1.Image)
	}

	i.Image, err = mutate.ConfigFile(i.Image, cfg)
	i.Image = mutate.Subject(i, *desc).(v1.Image)
	return err
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

func (b *PackageBuilder) SaveAsFile(path, version string, target dist.Target, labels map[string]string) error {
	if err := b.validate(); err != nil {
		return err
	}

	platform := target.Platform()
	platform.OSVersion = version
	layoutImage, err := newLayoutImage(*platform)
	if err != nil {
		return errors.Wrap(err, "creating layout image")
	}

	if platform.OS != "" {
		if err := layoutImage.SetOS(platform.OS); err != nil {
			return err
		}
	}

	if platform.Architecture != "" {
		if err := layoutImage.SetArchitecture(platform.Architecture); err != nil {
			return err
		}
	}

	if platform.Variant != "" {
		if err := layoutImage.SetVariant(platform.Variant); err != nil {
			return err
		}
	}

	if platform.OSVersion != "" {
		if err := layoutImage.SetOSVersion(platform.OSVersion); err != nil {
			return err
		}
	}

	if len(platform.Features) != 0 {
		if err := layoutImage.SetFeatures(platform.Features); err != nil {
			return err
		}
	}

	if len(platform.OSFeatures) != 0 {
		if err := layoutImage.SetOSFeatures(platform.OSFeatures); err != nil {
			return err
		}
	}

	if urls := target.URLs(); len(urls) != 0 {
		if err := layoutImage.SetURLs(urls); err != nil {
			return err
		}
	}

	if annos, err := target.Annotations(); len(annos) != 0 && err == nil {
		for k, v := range labels {
			annos[k] = v
		}

		if err := layoutImage.SetAnnotations(annos); err != nil {
			return err
		}
	}

	if err := layoutImage.Save(); err != nil {
		return err
	}

	for labelKey, labelValue := range labels {
		err = layoutImage.SetLabel(labelKey, labelValue)
		if err != nil {
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

func (b *PackageBuilder) SaveAsImage(repoName, version string, publish bool, target dist.Target, labels map[string]string) (imgutil.Image, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	image, err := b.imageFactory.NewImage(repoName, !publish, target)
	if err != nil {
		return nil, errors.Wrapf(err, "creating image")
	}

	for labelKey, labelValue := range labels {
		err = image.SetLabel(labelKey, labelValue)
		if err != nil {
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

	digest, err := getImageDigest(repoName, image)
	if err != nil {
		return nil, err
	}

	addtionalNames, err := getAddtionalImageNames(digest, target)
	if err != nil {
		return nil, err
	}

	if err := updateImagePlatform(image, target); err != nil {
		return nil, err
	}

	features, _ := image.Features()
	osFeatures, _ := image.OSFeatures()
	urls, _ := image.URLs()
	annotations, _ := image.Annotations()

	var featuresFound, osFeaturesFound, urlsFound, annosFound = true, true, true, true
	featuresFound = sliceContains(features, target.Specs.Features)
	osFeaturesFound = sliceContains(osFeatures, target.Specs.OSFeatures)
	urlsFound = sliceContains(urls, target.Specs.URLs)
	annosFound = mapContains(annotations, target.Specs.Annotations)
	if version != "" {
		if err := image.SetOSVersion(version); err != nil {
			return nil, err
		}
	}

	switch image.Kind() {
	case pkgImg.LOCAL:
	default:
		if !featuresFound {
			if err := image.SetFeatures(features); err != nil {
				return nil, err
			}
		}

		if !osFeaturesFound {
			if err := image.SetOSFeatures(osFeatures); err != nil {
				return nil, err
			}
		}

		if !urlsFound {
			if err := image.SetURLs(urls); err != nil {
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

			if err := image.SetAnnotations(annotations); err != nil {
				return nil, err
			}
		}
	}

	if err := image.Save(addtionalNames...); err != nil {
		return nil, err
	}

	idx, err := b.indexFactory.LoadIndex(repoName)
	if err != nil {
		return nil, err
	}

	if err := idx.Add(digest); err != nil {
		return nil, err
	}

	switch image.Kind() {
	case pkgImg.LOCAL:
	default:
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
	}

	if publish {
		if err := idx.Push(imgutil.WithInsecure(true)); err != nil {
			return nil, err
		}
	}

	return image, nil
}

func mapContains(src, conatins map[string]string) bool {
	for k, v := range conatins {
		if srcValue, ok := src[k]; !ok || srcValue != v {
			return false
		}
	}
	return true
}

func sliceContains(src, contains []string) bool {
	for _, c := range contains {
		found := false
		for _, srcString := range src {
			if c == srcString {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func updateImagePlatform(image imgutil.Image, target dist.Target) error {
	platform := target.Platform()

	if platform.OS != "" {
		if err := image.SetOS(platform.OS); err != nil {
			return err
		}
	}

	if platform.Architecture != "" {
		if err := image.SetArchitecture(platform.Architecture); err != nil {
			return err
		}
	}

	if platform.Variant != "" {
		if err := image.SetVariant(platform.Variant); err != nil {
			return err
		}
	}

	switch image.Kind() {
	case pkgImg.LOCAL:
		return nil
	default:
		if len(target.Specs.Features) > 0 {
			if err := image.SetFeatures(target.Specs.Features); err != nil {
				return err
			}
		}

		if len(target.Specs.OSFeatures) > 0 {
			if err := image.SetOSFeatures(target.Specs.OSFeatures); err != nil {
				return err
			}
		}

		if len(target.Specs.Annotations) > 0 {
			if err := image.SetAnnotations(target.Specs.Annotations); err != nil {
				return err
			}
		}

		return nil
	}
}

func getImageDigest(repoName string, image imgutil.Image) (digest name.Digest, err error) {
	ref, err := name.ParseReference(repoName)
	if err != nil {
		return digest, err
	}

	switch k := image.Kind(); k {
	case pkgImg.LOCAL_LAYOUT:
		fallthrough
	case pkgImg.LOCAL:
		id, err := image.Identifier()
		if err != nil {
			return digest, err
		}

		return ref.Context().Digest("sha256:" + id.String()), nil
	case pkgImg.LAYOUT:
		fallthrough
	case pkgImg.REMOTE:
		id, err := image.Identifier()
		if err != nil {
			return digest, err
		}
		return name.NewDigest(id.String(), name.Insecure, name.WeakValidation)
	default:
		// fmt.Errorf("unsupported image type: %s", k)
		return digest, nil
	}
}

func getAddtionalImageNames(name name.Reference, target dist.Target) ([]string, error) {
	hash, err := v1.NewHash(name.Identifier())
	if err != nil {
		return nil, err
	}

	return []string{
		hash.Hex,
		PlatformSafeName(name.Context().Name(), target),
	}, nil
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
