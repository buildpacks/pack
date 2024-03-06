package buildpackage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

const (
	platformDelim = "/"
	distroDelim   = "@"
	BuildpackToml = "buildpack.toml"
	PackageToml   = "package.toml"
)

type BuildpackType int

const (
	Buildpack BuildpackType = iota
	Composite
	Extention
)

type multiArchBuildpack struct {
	relativeBaseDir string
	flatten         bool
	flagTargets     []dist.Target
	config          dist.BuildpackDescriptor
	logger          logging.Logger
}

type multiArchPackage struct {
	relativeBaseDir string
	config          Config
	logger          logging.Logger
}

type multiArchBuildpackConfig struct {
	dist.BuildpackDescriptor
	dist.Platform
	bpType          BuildpackType
	relativeBaseDir string
	Flatten         bool
	FlattenExclude  []string
	Labels          map[string]string
}

func NewMultiArchBuildpack(config dist.BuildpackDescriptor, relativeBaseDir string, flatten bool, flags []dist.Target, logger logging.Logger) *multiArchBuildpack {
	if relativeBaseDir == "" {
		relativeBaseDir = "."
	}

	return &multiArchBuildpack{
		relativeBaseDir: relativeBaseDir,
		config:          config,
		flatten:         flatten,
		flagTargets:     flags,
		logger:          logger,
	}
}

func NewMultiArchPackage(config Config, relativeBaseDir string, logger logging.Logger) *multiArchPackage {
	if relativeBaseDir == "" {
		relativeBaseDir = "."
	}

	return &multiArchPackage{
		relativeBaseDir: relativeBaseDir,
		config:          config,
		logger:          logger,
	}
}

func (m *multiArchBuildpack) Targets() []dist.Target {
	if len(m.flagTargets) > 0 {
		return m.flagTargets
	}
	return m.config.WithTargets
}

func (m *multiArchBuildpack) MultiArchConfigs() (configs []multiArchBuildpackConfig) {
	targets := m.Targets()
	for _, target := range targets {
		for _, distro := range target.Distributions {
			for _, version := range distro.Versions {
				configs = append(configs, m.processTarget(target, distro, version))
			}
		}
	}
	return configs
}

func (m *multiArchBuildpack) processTarget(target dist.Target, distro dist.Distribution, version string) multiArchBuildpackConfig {
	bpType := Buildpack
	if len(m.config.WithOrder) > 0 {
		bpType = Composite
	}

	return multiArchBuildpackConfig{
		BuildpackDescriptor: dist.BuildpackDescriptor{
			WithInfo: m.config.WithInfo,
			WithTargets: []dist.Target{
				processTarget(target, distro, version),
			},
			WithAPI:          m.config.WithAPI,
			WithLinuxBuild:   m.config.WithLinuxBuild,
			WithWindowsBuild: m.config.WithWindowsBuild,
			WithStacks:       m.config.WithStacks,
			WithOrder:        m.config.WithOrder,
		},
		Platform:        processPlatform(target, distro, version),
		Flatten:         m.flatten || distro.Specs.Flatten,
		FlattenExclude:  distro.Specs.FlattenExclude,
		Labels:          distro.Specs.Labels,
		relativeBaseDir: m.relativeBaseDir,
		bpType:          bpType,
	}
}

func processTarget(target dist.Target, distro dist.Distribution, version string) dist.Target {
	return dist.Target{
		OS:          target.OS,
		Arch:        target.Arch,
		ArchVariant: target.ArchVariant,
		Distributions: []dist.Distribution{
			{
				Name:     distro.Name,
				Versions: []string{version},
				Specs: dist.TargetSpecs{
					Features:       distro.Specs.Features,
					OSFeatures:     distro.Specs.OSFeatures,
					URLs:           distro.Specs.URLs,
					Annotations:    distro.Specs.Annotations,
					Flatten:        distro.Specs.Flatten,
					FlattenExclude: distro.Specs.FlattenExclude,
					Labels:         distro.Specs.Labels,
				},
			},
		},
	}
}

func processPlatform(target dist.Target, distro dist.Distribution, version string) dist.Platform {
	return dist.Platform{
		OS: target.OS,
	}
}

func (m *multiArchBuildpackConfig) CopyBuildpackToml(getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (err error) {
	if uri := m.BuildpackDescriptor.WithInfo.ID; uri != "" {
		if m.BuildpackDescriptor.WithInfo, err = processModuleInfo(m.BuildpackDescriptor.WithInfo, m.relativeBaseDir, nil, nil); err != nil {
			return err
		}

		target := m.BuildpackDescriptor.WithTargets[0]
		distro := target.Distributions[0]
		bpFilePath := filepath.Join(platformRootDirectory(target, distro, distro.Versions[0]), BuildpackToml)
		for i, order := range m.WithOrder {
			for j, mg := range order.Group {
				if m.WithOrder[i].Group[j].ModuleInfo, err = processModuleInfo(mg.ModuleInfo, m.relativeBaseDir, &target, getIndexManifest); err != nil {
					return err
				}
			}
		}

		if m.bpType != Composite {
			bpFile, err := os.OpenFile(bpFilePath, os.O_WRONLY|os.O_CREATE, 0644)
			if err != nil {
				return err
			}
			return toml.NewEncoder(bpFile).Encode(m.BuildpackDescriptor)
		}

		return nil
	}
	return errors.New("invalid MultiArchBuildpackConfig")
}

func (m *multiArchBuildpackConfig) BuildpackType() BuildpackType {
	return m.bpType
}

func processModuleInfo(module dist.ModuleInfo, relativeBaseDir string, target *dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (m dist.ModuleInfo, err error) {
	if module.ID == "" {
		return module, errors.New("module id must be defined")
	}

	if module.Version == "" {
		module.Version = "latest"
	}

	module.ID, err = getRelativeUri(module.ID, relativeBaseDir, target, getIndexManifest)
	return module, err
}

func (m *multiArchBuildpackConfig) CleanBuildpackToml() error {
	target := m.BuildpackDescriptor.WithTargets[0]
	distro := target.Distributions[0]
	return os.Remove(filepath.Join(platformRootDirectory(target, distro, distro.Versions[0]), BuildpackToml))
}

func (m *multiArchPackage) CopyPackageToml(target dist.Target, distro dist.Distribution, version string, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (err error) {
	if m.config.Buildpack.URI != "" || m.config.Extension.URI != "" {
		platformRootDir := platformRootDirectory(target, distro, version)
		if uri := m.config.Buildpack.URI; uri != "" {
			if m.config.Buildpack.URI, err = getRelativeUri(uri, m.relativeBaseDir, &target, getIndexManifest); err != nil {
				return err
			}
		}

		if uri := m.config.Extension.URI; uri != "" {
			if m.config.Extension.URI, err = getRelativeUri(uri, m.relativeBaseDir, &target, getIndexManifest); err != nil {
				return err
			}
		}

		for i, dep := range m.config.Dependencies {
			// dep.ImageName == dep.ImageRef.ImageName, dep.URI == dep.Buildpack.URI
			if dep.URI != "" {
				if m.config.Dependencies[i].URI, err = getRelativeUri(dep.URI, m.relativeBaseDir, &target, getIndexManifest); err != nil {
					return err
				}
			}
		}

		bpFilePath := filepath.Join(platformRootDir, BuildpackToml)
		bpFile, err := os.OpenFile(bpFilePath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		return toml.NewEncoder(bpFile).Encode(m.config)
	}
	return nil
}

func (m *multiArchPackage) Config() Config {
	return m.config
}

func getRelativeUri(uri, relativeBaseDir string, target *dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (string, error) {
	locator, err := buildpack.GetLocatorType(uri, relativeBaseDir, []dist.ModuleInfo{})
	if err != nil {
		return "", err
	}

	switch locator {
	case buildpack.URILocator:
		return paths.FilePathToURI(uri, relativeBaseDir)
	case buildpack.DockerLocalIndex:
		if target == nil {
			return "", fmt.Errorf("nil target")
		}
		ref, err := parseURItoString(uri, *target, getIndexManifest)
		return "docker://" + ref, err
	case buildpack.OCILocalIndex:
		if target == nil {
			return "", fmt.Errorf("nil target")
		}
		ref, err := parseURItoString(uri, *target, getIndexManifest)
		return ref + ".cnb", err
	default:
		return uri, nil
	}
}

func parseURItoString(uri string, target dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (string, error) {
	loc := strings.SplitN(uri, "//", 2)
	if len(loc) > 1 {
		ref, err := name.ParseReference(loc[1], name.Insecure, name.WeakValidation)
		if err != nil {
			return "", err
		}

		idx, err := getIndexManifest(ref)
		if err != nil {
			return "", err
		}
		return HexFromIndex(idx, target)
	}
	return "", fmt.Errorf("invalid uri: %s", style.Symbol(uri))
}

func (m *multiArchPackage) CleanPackageToml(target dist.Target, distro dist.Distribution, version string) error {
	return os.Remove(filepath.Join(platformRootDirectory(target, distro, version), PackageToml))
}

func HexFromIndex(idx *v1.IndexManifest, target dist.Target) (string, error) {
	if idx == nil {
		return "", imgutil.ErrManifestUndefined
	}

	for _, mfest := range idx.Manifests {
		if mfest.Platform == nil {
			return "", imgutil.ErrPlatformUndefined
		}

		platform := mfest.Platform
		if platform.Satisfies(*target.Platform()) {
			return mfest.Digest.Hex, nil
		}
	}

	return "", errors.New("no image found with given platform")
}
