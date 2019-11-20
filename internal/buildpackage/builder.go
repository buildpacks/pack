package buildpackage

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/internal/dist"
	"github.com/buildpack/pack/internal/style"
)

type ImageFactory interface {
	NewImage(repoName string, local bool) (imgutil.Image, error)
}

type PackageBuilder struct {
	defaultBpInfo dist.BuildpackInfo
	buildpacks    []dist.Buildpack
	packages      []Package
	stacks        []dist.Stack
	imageFactory  ImageFactory
}

type Package interface {
	Name() string
	BuildpackLayers() dist.BuildpackLayers
	GetLayer(diffID string) (io.ReadCloser, error)
}

func NewBuilder(imageFactory ImageFactory) *PackageBuilder {
	return &PackageBuilder{
		imageFactory: imageFactory,
	}
}

func (p *PackageBuilder) SetDefaultBuildpack(bpInfo dist.BuildpackInfo) {
	p.defaultBpInfo = bpInfo
}

func (p *PackageBuilder) AddBuildpack(buildpack dist.Buildpack) {
	p.buildpacks = append(p.buildpacks, buildpack)
}

func (p *PackageBuilder) AddPackage(pkg Package) {
	p.packages = append(p.packages, pkg)
}

func (p *PackageBuilder) AddStack(stack dist.Stack) {
	p.stacks = append(p.stacks, stack)
}

func (p *PackageBuilder) Save(repoName string, publish bool) (imgutil.Image, error) {
	var bpds []dist.BuildpackDescriptor
	for _, bp := range p.buildpacks {
		bpds = append(bpds, bp.Descriptor())
	}

	for _, pkgImage := range p.packages {
		for bpID, v := range pkgImage.BuildpackLayers() {
			for bpVersion, bpInfo := range v {
				bpds = append(bpds, dist.BuildpackDescriptor{
					API: bpInfo.API,
					Info: dist.BuildpackInfo{
						ID:      bpID,
						Version: bpVersion,
					},
					Stacks: bpInfo.Stacks,
					Order:  bpInfo.Order,
				})
			}
		}
	}

	if err := validateDefault(bpds, p.defaultBpInfo); err != nil {
		return nil, err
	}

	if err := validateStacks(bpds, p.stacks); err != nil {
		return nil, err
	}

	image, err := p.imageFactory.NewImage(repoName, !publish)
	if err != nil {
		return nil, errors.Wrapf(err, "creating image")
	}

	if err := dist.SetLabel(image, MetadataLabel, &Metadata{
		BuildpackInfo: p.defaultBpInfo,
		Stacks:        p.stacks,
	}); err != nil {
		return nil, err
	}

	tmpDir, err := ioutil.TempDir("", "create-package")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	bpLayers := dist.BuildpackLayers{}
	for _, bp := range p.buildpacks {
		bpLayerTar, err := dist.BuildpackLayer(tmpDir, 0, 0, bp)
		if err != nil {
			return nil, err
		}

		if err := image.AddLayer(bpLayerTar); err != nil {
			return nil, errors.Wrapf(err, "adding layer tar for buildpack %s", style.Symbol(bp.Descriptor().Info.FullName()))
		}

		diffID, err := dist.LayerDiffID(bpLayerTar)
		if err != nil {
			return nil, errors.Wrapf(err,
				"getting content hashes for buildpack %s",
				style.Symbol(bp.Descriptor().Info.FullName()),
			)
		}

		bpInfo := bp.Descriptor().Info
		if _, ok := bpLayers[bpInfo.ID]; !ok {
			bpLayers[bpInfo.ID] = map[string]dist.BuildpackLayerInfo{}
		}
		bpLayers[bpInfo.ID][bpInfo.Version] = dist.BuildpackLayerInfo{
			API:         bp.Descriptor().API,
			Stacks:      bp.Descriptor().Stacks,
			Order:       bp.Descriptor().Order,
			LayerDiffID: diffID.String(),
		}
	}

	// add bps from packages
	for _, pkg := range p.packages {
		for bpID, v := range pkg.BuildpackLayers() {
			for bpVersion, bpInfo := range v {
				if err := embedBuildpackToImage(pkg, bpInfo, tmpDir, image); err != nil {
					return nil, errors.Wrapf(err, "embedding buildpack %s", style.Symbol(bpID+"@"+bpVersion))
				}

				if _, ok := bpLayers[bpID]; !ok {
					bpLayers[bpID] = map[string]dist.BuildpackLayerInfo{}
				}

				bpLayers[bpID][bpVersion] = bpInfo
			}
		}
	}

	if err := dist.SetLabel(image, dist.BuildpackLayersLabel, bpLayers); err != nil {
		return nil, err
	}

	if err := image.Save(); err != nil {
		return nil, err
	}

	return image, nil
}

func embedBuildpackToImage(pkg Package, bpInfo dist.BuildpackLayerInfo, tmpDir string, image imgutil.Image) error {
	readCloser, err := pkg.GetLayer(bpInfo.LayerDiffID)
	if err != nil {
		return errors.Wrap(err, "retrieve layer")
	}
	defer readCloser.Close()

	file, err := ioutil.TempFile(tmpDir, "*.tar")
	if err != nil {
		return errors.Wrap(err, "creating temp file")
	}
	if _, err = io.Copy(file, readCloser); err != nil {
		return errors.Wrap(err, "copy layer contents")
	}

	if err = image.AddLayer(file.Name()); err != nil {
		return errors.Wrap(err, "adding layer")
	}
	return nil
}

func validateDefault(bps []dist.BuildpackDescriptor, defBp dist.BuildpackInfo) error {
	if defBp.ID == "" || defBp.Version == "" {
		return errors.New("a default buildpack must be set")
	}

	if !bpExists(bps, defBp) {
		return errors.Errorf("selected default %s is not present",
			style.Symbol(defBp.FullName()),
		)
	}

	return nil
}

func validateStacks(bps []dist.BuildpackDescriptor, stacks []dist.Stack) error {
	if len(stacks) == 0 {
		return errors.New("must specify at least one supported stack")
	}

	declaredStacks := map[string]interface{}{}
	for _, s := range stacks {
		if _, ok := declaredStacks[s.ID]; ok {
			return errors.Errorf("stack %s was specified more than once", style.Symbol(s.ID))
		}

		declaredStacks[s.ID] = nil

		for _, bpd := range bps {
			if err := bpd.EnsureStackSupport(s.ID, s.Mixins, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func bpExists(bps []dist.BuildpackDescriptor, search dist.BuildpackInfo) bool {
	for _, bpd := range bps {
		if bpd.Info.ID == search.ID && bpd.Info.Version == search.Version {
			return true
		}
	}

	return false
}
