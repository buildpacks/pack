package buildpackage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"

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

type MultiArchBuildpack struct {
	name        name.Tag
	bpTargets   []dist.Target
	flagTargets []dist.Target
	config dist.BuildpackDescriptor
	logger      logging.Logger
}

type MultiArchPackage struct {
	pkgTargets  []dist.Target
	flagTargets []dist.Target
	relativeBaseDir string
	config      Config
	logger      logging.Logger
}

type MultiArchBuildpackConfig struct {
	dist.BuildpackDescriptor
	dist.Platform
	Flatten        bool
	FlattenExclude []string
	Labels         map[string]string
}

type MultiArchPackageConfig struct {
	Config
	Flatten        bool
	FlattenExclude []string
	Labels         map[string]string
	relativeBaseDir string
}

func NewMultiArchBuildpack(config ,name name.Tag, bp []dist.Target, flags []dist.Target, logger logging.Logger) MultiArchBuildpack {
	return MultiArchBuildpack{
		name:        name,
		bpTargets:   bp,
		flagTargets: flags,
		logger:      logger,
	}
}

func NewMultiArchPackage(config Config, relativeBaseDir string, pkg []dist.Target, flags []dist.Target, logger logging.Logger) MultiArchPackage {
	return MultiArchPackage{
		relativeBaseDir: relativeBaseDir,
		config:      config,
		pkgTargets:  pkg,
		flagTargets: flags,
		logger:      logger,
	}
}

func (m *MultiArchBuildpack) Targets() []dist.Target {
	if len(m.flagTargets) > 0 {
		return m.flagTargets
	}
	return m.bpTargets
}

func (m *MultiArchPackage) Targets() []dist.Target {
	if len(m.flagTargets) > 0 {
		return m.flagTargets
	}
	return m.pkgTargets
}

func (m *MultiArchBuildpack) MultiArchConfigs() (configs []MultiArchBuildpackConfig) {
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

func (m *MultiArchPackage) MultiArchConfigs() (configs []MultiArchPackageConfig) {
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

func (m *MultiArchBuildpack) processTarget(target dist.Target, distro dist.Distribution, version string) MultiArchBuildpackConfig {
	return MultiArchBuildpackConfig{
		BuildpackDescriptor: dist.BuildpackDescriptor{
			WithInfo: m.config.WithInfo,
			WithTargets: []dist.Target{
				processTarget(target, distro, version),
			},
			WithAPI:          m.config.WithAPI,
			WithLinuxBuild:   m.config.WithLinuxBuild,
			WithWindowsBuild: m.config.WithWindowsBuild,
			WithStacks: m.config.WithStacks,
			WithOrder: m.config.WithOrder,
		},
		Platform:       processPlatform(target, distro, version),
		Flatten:        distro.Specs.Flatten,
		FlattenExclude: distro.Specs.FlattenExclude,
		Labels:         distro.Specs.Labels,
	}
}

func (m *MultiArchPackage) processTarget(target dist.Target, distro dist.Distribution, version string) MultiArchPackageConfig {
	return MultiArchPackageConfig{
		Config: Config{
			Buildpack:    m.config.Buildpack,
			Extension:    m.config.Extension,
			Dependencies: m.config.Dependencies,
			Targets: []dist.Target{
				processTarget(target, distro, version),
			},
			Platform: processPlatform(target, distro, version),
		},
		Flatten:        distro.Specs.Flatten,
		FlattenExclude: distro.Specs.FlattenExclude,
		Labels:         distro.Specs.Labels,
		relativeBaseDir: m.relativeBaseDir,
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
		OS:          target.OS,
		Arch:        target.Arch,
		Variant:     target.ArchVariant,
		OSVersion:   version,
		Features:    distro.Specs.Features,
		OSFeatures:  distro.Specs.OSFeatures,
		URLs:        distro.Specs.URLs,
		Annotations: distro.Specs.Annotations,
	}
}

func (m *MultiArchBuildpackConfig) CopyBuildpackToml() error {
	if m.BuildpackDescriptor.WithInfo.ID != "" {
		target := m.BuildpackDescriptor.WithTargets[0]
		distro := target.Distributions[0]
		bpFilePath := filepath.Join(platformRootDirectory(target, distro, distro.Versions[0]), BuildpackToml)
		bpFile, err := os.OpenFile(bpFilePath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		return toml.NewEncoder(bpFile).Encode(m.BuildpackDescriptor)
	}
	return errors.New("invalid MultiArchBuildpackConfig")
}

func (m *MultiArchBuildpackConfig) CleanBuildpackToml() error {
	target := m.BuildpackDescriptor.WithTargets[0]
	distro := target.Distributions[0]
	return os.Remove(filepath.Join(platformRootDirectory(target, distro, distro.Versions[0]), BuildpackToml))
}

func (m *MultiArchPackageConfig) CopyPackageToml() (err error) {
	if m.Config.Buildpack.URI != "" || m.Config.Extension.URI != "" {
		target := m.Config.Targets[0]
		distro := target.Distributions[0]
		platformRootDir := platformRootDirectory(target, distro, distro.Versions[0])
		if uri := m.Config.Buildpack.URI; uri != "" {
			if m.Config.Buildpack.URI, err = getRelativeUri(uri, m.relativeBaseDir); err != nil {
				return err
			}
		}

		if uri := m.Config.Extension.URI; uri != "" {
			if m.Config.Extension.URI, err = getRelativeUri(uri, m.relativeBaseDir); err != nil {
				return err
			}
		}

		bpFilePath := filepath.Join(platformRootDir, BuildpackToml)
		bpFile, err := os.OpenFile(bpFilePath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		return toml.NewEncoder(bpFile).Encode(m.Config)
	}
	return nil
}

func getRelativeUri(uri, relativeBaseDir string) (string, error) {
	locator, err := buildpack.GetLocatorType(uri, relativeBaseDir, []dist.ModuleInfo{})
	if err != nil {
		return "", err
	}

	if locator == buildpack.URILocator {
		uri, err = paths.FilePathToURI(uri, relativeBaseDir)
		if err != nil {
			return "", fmt.Errorf("making absolute: %s:\n %s", style.Symbol(uri), err.Error())
		}
	}
	return uri, nil
}

func (m *MultiArchPackageConfig) CleanPackageToml() error {
	target := m.Config.Targets[0]
	distro := target.Distributions[0]
	return os.Remove(filepath.Join(platformRootDirectory(target, distro, distro.Versions[0]), PackageToml))
}
