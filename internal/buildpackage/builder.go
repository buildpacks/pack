package buildpackage

import (
	"io/ioutil"
	"os"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/internal/style"
)

type ImageFactory interface {
	NewImage(repoName string, local bool) (imgutil.Image, error)
}

type PackageBuilder struct {
	defaultBpInfo dist.BuildpackInfo
	buildpacks    []dist.Buildpack
	stacks        []dist.Stack
	imageFactory  ImageFactory
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

func (p *PackageBuilder) AddStack(stack dist.Stack) {
	p.stacks = append(p.stacks, stack)
}

func (p *PackageBuilder) Save(repoName string, publish bool) (imgutil.Image, error) {
	var bpds []dist.BuildpackDescriptor
	for _, bp := range p.buildpacks {
		bpds = append(bpds, bp.Descriptor())
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
		bpLayerTar, err := dist.BuildpackToLayerTar(tmpDir, bp)
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

		dist.AddBuildpackToLayersMD(bpLayers, bp.Descriptor(), diffID.String())
	}

	if err := dist.SetLabel(image, dist.BuildpackLayersLabel, bpLayers); err != nil {
		return nil, err
	}

	if err := image.Save(); err != nil {
		return nil, err
	}

	return image, nil
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
