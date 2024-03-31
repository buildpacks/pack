package buildpackage

import (
	"errors"
	"fmt"
	"net/url"
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
	DigestDelim   = "@"
	TagDelim      = ":"
	RegistryDelim = "/"
	PackageToml   = "package.toml"
	BuildpackToml = "buildpack.toml"
)

type BuildpackType int
type GetIndexManifestFn func(ref name.Reference) (*v1.IndexManifest, error)

const (
	Buildpack BuildpackType = iota
	Composite
	Extension
)

type multiArchBuildpack struct {
	relativeBaseDir string
	flatten         bool
	flagTargets     []dist.Target
	config          dist.BuildpackDescriptor
	flattenChanged  bool
}

type multiArchExtension struct {
	config          dist.ExtensionDescriptor
	flagTargets     []dist.Target
	relativeBaseDir string
}

type IndexOptions struct {
	BPConfigs       *[]MultiArchBuildpackConfig
	ExtConfigs      *[]MultiArchExtensionConfig
	PkgConfig       *MultiArchPackage
	Logger          logging.Logger
	RelativeBaseDir string
	Target          dist.Target
	ImageIndex      imgutil.ImageIndex
}

type MultiArchPackage struct {
	Config
	relativeBaseDir string
}

type MultiArchBuildpackConfig struct {
	dist.BuildpackDescriptor
	dist.Platform
	bpType          BuildpackType
	relativeBaseDir string
	Flatten         bool
	FlattenExclude  []string
	Labels          map[string]string
}

type MultiArchExtensionConfig struct {
	dist.ExtensionDescriptor
	dist.Platform
	relativeBaseDir string
}

func NewMultiArchBuildpack(config dist.BuildpackDescriptor, relativeBaseDir string, flatten, flattenChanged bool, flags []dist.Target) *multiArchBuildpack {
	if relativeBaseDir == "" {
		relativeBaseDir = "."
	}

	return &multiArchBuildpack{
		relativeBaseDir: relativeBaseDir,
		config:          config,
		flatten:         flatten,
		flagTargets:     flags,
		flattenChanged:  flattenChanged,
	}
}

func NewMultiArchExtension(config dist.ExtensionDescriptor, relativeBaseDir string, flags []dist.Target) *multiArchExtension {
	if relativeBaseDir == "" {
		relativeBaseDir = "."
	}

	return &multiArchExtension{
		config:          config,
		flagTargets:     flags,
		relativeBaseDir: relativeBaseDir,
	}
}

func NewMultiArchPackage(config Config, relativeBaseDir string) *MultiArchPackage {
	if relativeBaseDir == "" {
		relativeBaseDir = "."
	}

	return &MultiArchPackage{
		relativeBaseDir: relativeBaseDir,
		Config:          config,
	}
}

func (m *multiArchBuildpack) Targets() []dist.Target {
	if len(m.flagTargets) > 0 {
		return m.flagTargets
	}
	return m.config.WithTargets
}

func (m *multiArchExtension) Targets() []dist.Target {
	if len(m.flagTargets) > 0 {
		return m.flagTargets
	}
	return m.config.WithTargets
}

func (m *multiArchBuildpack) MultiArchConfigs() (configs []MultiArchBuildpackConfig, err error) {
	targets := m.Targets()
	for _, target := range targets {
		if len(target.Distributions) == 0 {
			cfg, err := m.processTarget(target, dist.Distribution{}, "")
			if err != nil {
				return configs, err
			}
			configs = append(configs, cfg)
		} else {
			for _, distro := range target.Distributions {
				if len(distro.Versions) == 0 {
					cfg, err := m.processTarget(target, distro, "")
					if err != nil {
						return configs, err
					}
					configs = append(configs, cfg)
				} else {
					for _, version := range distro.Versions {
						cfg, err := m.processTarget(target, distro, version)
						if err != nil {
							return configs, err
						}
						configs = append(configs, cfg)
					}
				}
			}
		}
	}
	return configs, nil
}

func (m *multiArchExtension) MultiArchConfigs() (configs []MultiArchExtensionConfig, err error) {
	targets := m.Targets()
	for _, target := range targets {
		if len(target.Distributions) == 0 {
			cfg, err := m.processTarget(target, dist.Distribution{}, "")
			if err != nil {
				return configs, err
			}
			configs = append(configs, cfg)
		} else {
			for _, distro := range target.Distributions {
				if len(distro.Versions) == 0 {
					cfg, err := m.processTarget(target, distro, "")
					if err != nil {
						return configs, err
					}
					configs = append(configs, cfg)
				} else {
					for _, version := range distro.Versions {
						cfg, err := m.processTarget(target, distro, version)
						if err != nil {
							return configs, err
						}
						configs = append(configs, cfg)
					}
				}
			}
		}
	}
	return configs, nil
}

func (m *multiArchBuildpack) processTarget(target dist.Target, distro dist.Distribution, version string) (MultiArchBuildpackConfig, error) {
	bpType := Buildpack
	if len(m.config.WithOrder) > 0 {
		bpType = Composite
	}

	if m.config.WithInfo.Version != "" {
		target.Specs.OSVersion = m.config.WithInfo.Version
	}

	rel, err := filepath.Abs(filepath.Join(m.relativeBaseDir, buildpack.PlatformRootDirectory(target, distro.Name, version), "buildpack.toml"))
	if err != nil {
		return MultiArchBuildpackConfig{}, err
	}

	return MultiArchBuildpackConfig{
		BuildpackDescriptor: dist.BuildpackDescriptor{
			WithInfo: m.config.WithInfo,
			WithTargets: []dist.Target{
				ProcessTarget(target, distro, version),
			},
			WithAPI:          m.config.WithAPI,
			WithLinuxBuild:   m.config.WithLinuxBuild,
			WithWindowsBuild: m.config.WithWindowsBuild,
			WithStacks:       m.config.WithStacks,
			WithOrder:        m.config.WithOrder,
		},
		Platform:        dist.Platform{OS: target.OS},
		Flatten:         (m.flattenChanged && m.flatten) || (!m.flattenChanged && target.Specs.Flatten), // let the flag value take precedence over the config
		FlattenExclude:  target.Specs.FlattenExclude,
		Labels:          target.Specs.Labels,
		relativeBaseDir: rel,
		bpType:          bpType,
	}, nil
}

func (m *multiArchExtension) processTarget(target dist.Target, distro dist.Distribution, version string) (MultiArchExtensionConfig, error) {
	if m.config.WithInfo.Version != "" {
		target.Specs.OSVersion = m.config.WithInfo.Version
	}

	rel, err := filepath.Abs(filepath.Join(m.relativeBaseDir, buildpack.PlatformRootDirectory(target, distro.Name, version), "extension.toml"))
	if err != nil {
		return MultiArchExtensionConfig{}, err
	}

	return MultiArchExtensionConfig{
		ExtensionDescriptor: dist.ExtensionDescriptor{
			WithInfo: m.config.WithInfo,
			WithTargets: []dist.Target{
				ProcessTarget(target, distro, version),
			},
			WithAPI: m.config.WithAPI,
		},
		Platform:        dist.Platform{OS: target.OS},
		relativeBaseDir: rel,
	}, nil
}

func ProcessTarget(target dist.Target, distro dist.Distribution, version string) dist.Target {
	return dist.Target{
		OS:          target.OS,
		Arch:        target.Arch,
		ArchVariant: target.ArchVariant,
		Specs:       target.Specs,
		Distributions: []dist.Distribution{
			{
				Name:     distro.Name,
				Versions: []string{version},
			},
		},
	}
}

func (m *MultiArchBuildpackConfig) Path() string {
	var target dist.Target
	targets := m.WithTargets
	if len(targets) != 0 {
		target = targets[0]
	}

	if path := target.Specs.Path; path != "" {
		return filepath.Join(path, "buildpack.toml")
	}

	return m.relativeBaseDir
}

func (m *MultiArchExtensionConfig) Path() string {
	var target dist.Target
	targets := m.WithTargets
	if len(targets) != 0 {
		target = targets[0]
	}

	if path := target.Specs.Path; path != "" {
		return filepath.Join(path, "extension.toml")
	}

	return m.relativeBaseDir
}

func (m *MultiArchBuildpackConfig) CopyBuildpackToml(getIndexManifest GetIndexManifestFn) (err error) {
	if uri := m.BuildpackDescriptor.WithInfo.ID; uri == "" {
		return errors.New("invalid MultiArchBuildpackConfig")
	}

	writeBPPath := m.Path()
	if err := os.MkdirAll(filepath.Dir(writeBPPath), os.ModePerm); err != nil {
		return err
	}

	bpFile, err := os.Create(writeBPPath)
	if err != nil {
		return err
	}

	return toml.NewEncoder(bpFile).Encode(m.BuildpackDescriptor)
}

func (m *MultiArchExtensionConfig) CopyExtensionToml(getIndexManifest GetIndexManifestFn) (err error) {
	if uri := m.ExtensionDescriptor.WithInfo.ID; uri == "" {
		return errors.New("invalid MultiArchBuildpackConfig")
	}

	writeExtPath := m.Path()
	if err := os.MkdirAll(filepath.Dir(writeExtPath), os.ModePerm); err != nil {
		return err
	}

	extFile, err := os.Create(writeExtPath)
	if err != nil {
		return err
	}

	return toml.NewEncoder(extFile).Encode(m.ExtensionDescriptor)
}

func (m *MultiArchBuildpackConfig) BuildpackType() BuildpackType {
	return m.bpType
}

func (m *MultiArchBuildpackConfig) RelativeBaseDir() string {
	return m.relativeBaseDir
}

func (m *MultiArchExtensionConfig) RelativeBaseDir() string {
	return m.relativeBaseDir
}

func (m *MultiArchBuildpackConfig) CleanBuildpackToml() error {
	return os.Remove(m.Path())
}

func (m *MultiArchExtensionConfig) CleanExtensionToml() error {
	return os.Remove(m.Path())
}

func (m *MultiArchPackage) RelativeBaseDir() string {
	return m.relativeBaseDir
}

func (m *MultiArchPackage) CopyPackageToml(relativeTo string, target dist.Target, distroName, version string, getIndexManifest GetIndexManifestFn) (err error) {
	multiArchPKGConfig := *m
	if (multiArchPKGConfig.Buildpack.URI == "" && multiArchPKGConfig.Extension.URI == "") || (multiArchPKGConfig.Buildpack.URI != "" && multiArchPKGConfig.Extension.URI != "") {
		return errors.New("unexpected: one of Buildpack URI, Extension URI must be specified")
	}

	if uri := multiArchPKGConfig.Buildpack.URI; uri != "" {
		if multiArchPKGConfig.Buildpack.URI, err = GetRelativeURI(uri, multiArchPKGConfig.relativeBaseDir, &target, getIndexManifest); err != nil {
			return err
		}
	}

	if uri := multiArchPKGConfig.Extension.URI; uri != "" {
		if multiArchPKGConfig.Extension.URI, err = GetRelativeURI(uri, multiArchPKGConfig.relativeBaseDir, &target, getIndexManifest); err != nil {
			return err
		}
	}

	for i, dep := range multiArchPKGConfig.Dependencies {
		// dep.ImageName == dep.ImageRef.ImageName, dep.URI == dep.Buildpack.URI
		if dep.URI != "" {
			if m.Dependencies[i].URI, err = GetRelativeURI(dep.URI, multiArchPKGConfig.relativeBaseDir, &target, getIndexManifest); err != nil {
				return err
			}
		}
	}

	platformRootDir := buildpack.PlatformRootDirectory(target, distroName, version)
	path, err := filepath.Abs(filepath.Join(relativeTo, platformRootDir, PackageToml))
	if err != nil {
		return err
	}

	// create parent folder if not exists
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}

	bpFile, err := os.Create(path)
	if err != nil {
		return err
	}
	return toml.NewEncoder(bpFile).Encode(multiArchPKGConfig.Config)
}

func GetRelativeURI(uri, relativeBaseDir string, target *dist.Target, getIndexManifest GetIndexManifestFn) (string, error) {
	locator, err := buildpack.GetLocatorType(uri, relativeBaseDir, []dist.ModuleInfo{})
	if err != nil {
		return "", err
	}

	switch locator {
	case buildpack.URILocator:
		// returns file://<file-name>-[os][-arch][-variant]-[name@version]
		// for multi-arch we need target specific path appended at the end of name

		if URL, err := url.Parse(uri); err == nil && URL.Host != "" {
			err = platformSafeURL(URL, target)
			return URL.String(), err
		}

		uri, err = filepath.Abs(filepath.Join(relativeBaseDir, uri))
		if err != nil {
			return "", err
		}

		target := *target
		distro := dist.Distribution{}
		if len(target.Distributions) > 0 {
			distro = target.Distributions[0]
		}
		version := ""
		if len(distro.Versions) > 0 {
			version = distro.Versions[0]
		}

		// uri = buildpack.PlatformSafeName(uri, target)
		return paths.FilePathToURI(filepath.Join(uri, buildpack.PlatformRootDirectory(target, distro.Name, version)), "")
	case buildpack.PackageLocator:
		if target == nil {
			return "", fmt.Errorf("nil target")
		}
		ref, err := ParseURItoString(buildpack.ParsePackageLocator(uri), *target, getIndexManifest)
		return "docker://" + ref, err
	case buildpack.RegistryLocator:
		if target == nil {
			return "", fmt.Errorf("nil target")
		}

		rNS, rName, rVersion, err := buildpack.ParseRegistryID(uri)
		if err != nil {
			return uri, err
		}

		ref, err := ParseURItoString(rNS+RegistryDelim+rName+DigestDelim+rVersion, *target, getIndexManifest)
		return "urn:cnb:registry:" + ref, err
	case buildpack.FromBuilderLocator:
		fallthrough
		// return uri, fmt.Errorf("buildpacks doesn't support '%s' or '%s'", "from=builder", "urn:cnb:builder")
	case buildpack.IDLocator:
		fallthrough
		// return uri, fmt.Errorf("buildpacks doesn't support '%s'", "IDLocator")
	// case buildpack.InvalidLocator:
	// 	return uri, fmt.Errorf("%s: '%s' not supported", "InvalidLocator", uri)
	default:
		return uri, nil
	}
}

func platformSafeURL(url *url.URL, target *dist.Target) error {
	if url == nil || target == nil {
		return fmt.Errorf("%s and %s must not be nil", "url", "target")
	}

	paths := strings.Split(url.Path, "/")
	lastSegment := paths[len(paths)-1]
	if lastSegmentPath, lastSegmentExt, found := strings.Cut(lastSegment, "."); found && lastSegmentExt != "" {
		lastSegment = buildpack.PlatformSafeName(lastSegmentPath, *target) + "." + lastSegmentExt
	} else {
		lastSegment = buildpack.PlatformSafeName(lastSegmentPath, *target)
	}

	url.Path = strings.Join(paths[:len(paths)-1], "/") + "/" + lastSegment
	return nil
}

func ParseURItoString(uri string, target dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (string, error) {
	if strings.Contains(uri, DigestDelim) {
		ref := strings.Split(uri, DigestDelim)
		registry := ref[0]
		hashStr := ref[1]
		if _, err := v1.NewHash(hashStr); err != nil {
			uri = registry + TagDelim + hashStr
		}
	}

	ref, err := name.ParseReference(uri, name.Insecure, name.WeakValidation)
	if err != nil {
		return "", err
	}

	idx, err := getIndexManifest(ref)
	if err != nil {
		return "", err
	}

	if idx == nil {
		return "", imgutil.ErrManifestUndefined
	}

	fmt.Printf("fetching image from repo: %s \n", style.Symbol(uri))
	digest, err := DigestFromIndex(idx, target)
	if err != nil {
		return "", err
	}

	return ref.Context().Digest(digest).Name(), nil
}

func (m *MultiArchPackage) CleanPackageToml(relativeTo string, target dist.Target, distroName, version string) error {
	path, err := filepath.Abs(filepath.Join(relativeTo, buildpack.PlatformRootDirectory(target, distroName, version), PackageToml))
	if err != nil {
		return err
	}

	return os.Remove(path)
}

func DigestFromIndex(idx *v1.IndexManifest, target dist.Target) (string, error) {
	if idx == nil {
		return "", imgutil.ErrManifestUndefined
	}

	targetPlatform := *target.Platform()
	for _, mfest := range idx.Manifests {
		if mfest.Platform == nil {
			return "", imgutil.ErrPlatformUndefined
		}

		platform := mfest.Platform
		if platform.Satisfies(targetPlatform) {
			return mfest.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("no image found for given platform %s", style.Symbol(fmt.Sprintf("%s%s%s", targetPlatform.OS, "/"+targetPlatform.Architecture, "/"+targetPlatform.Variant)))
}
