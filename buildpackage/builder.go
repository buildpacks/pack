package buildpackage

import (
	"io/ioutil"
	"os"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/dist"
	"github.com/buildpack/pack/style"
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
	if err := validateDefault(p.buildpacks, p.defaultBpInfo); err != nil {
		return nil, err
	}

	if err := validateStacks(p.buildpacks, p.stacks); err != nil {
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

	for _, bp := range p.buildpacks {
		bpLayerTar, err := dist.BuildpackLayer(tmpDir, 0, 0, bp)
		if err != nil {
			return nil, err
		}

		if err := image.AddLayer(bpLayerTar); err != nil {
			return nil, errors.Wrapf(err, "adding layer tar for buildpack %s", style.Symbol(bp.Descriptor().Info.FullName()))
		}
	}

	if err := image.Save(); err != nil {
		return nil, err
	}

	return image, nil
}

func validateDefault(bps []dist.Buildpack, defBp dist.BuildpackInfo) error {
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

func validateStacks(bps []dist.Buildpack, stacks []dist.Stack) error {
	if len(stacks) == 0 {
		return errors.New("must specify at least one supported stack")
	}

	declaredStacks := map[string]interface{}{}
	for _, s := range stacks {
		if _, ok := declaredStacks[s.ID]; ok {
			return errors.Errorf("stack %s was specified more than once", style.Symbol(s.ID))
		}

		declaredStacks[s.ID] = nil

		for _, bp := range bps {
			bpd := bp.Descriptor()
			if err := bpd.EnsureStackSupport(s.ID, s.Mixins, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func bpExists(bps []dist.Buildpack, search dist.BuildpackInfo) bool {
	for _, bp := range bps {
		bpInfo := bp.Descriptor().Info
		if bpInfo.ID == search.ID && bpInfo.Version == search.Version {
			return true
		}
	}

	return false
}
