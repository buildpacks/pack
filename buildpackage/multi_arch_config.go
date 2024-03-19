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

type IndexOptions struct {
	BPConfigs       *[]MultiArchBuildpackConfig
	PkgConfig       *MultiArchPackage
	Manifest        *v1.IndexManifest
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

func (m *multiArchBuildpack) MultiArchConfigs() (configs []MultiArchBuildpackConfig, err error) {
	targets := m.Targets()
	for _, target := range targets {
		for _, distro := range target.Distributions {
			for _, version := range distro.Versions {
				cfg, err := m.processTarget(target, distro, version)
				if err != nil {
					return configs, err
				}
				configs = append(configs, cfg)
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

	rel, err := filepath.Abs(filepath.Join(m.relativeBaseDir, buildpack.PlatformRootDirectory(target, distro.Name, version), "buildpack.toml"))
	if err != nil {
		return MultiArchBuildpackConfig{}, err
	}

	return MultiArchBuildpackConfig{
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
		Platform:        dist.Platform{OS: target.OS},
		Flatten:         (m.flattenChanged && m.flatten) || (!m.flattenChanged && target.Specs.Flatten), // let the flag value take precedence over the config
		FlattenExclude:  target.Specs.FlattenExclude,
		Labels:          target.Specs.Labels,
		relativeBaseDir: rel,
		bpType:          bpType,
	}, nil
}

func processTarget(target dist.Target, distro dist.Distribution, version string) dist.Target {
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

func (m *MultiArchBuildpackConfig) CopyBuildpackToml(getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (err error) {
	if uri := m.BuildpackDescriptor.WithInfo.ID; uri != "" {
		// if m.BuildpackDescriptor.WithInfo, err = processModuleInfo(m.BuildpackDescriptor.WithInfo, m.relativeBaseDir, nil, nil); err != nil {
		// 	return err
		// }

		// target := m.BuildpackDescriptor.WithTargets[0]
		// for i, order := range m.WithOrder {
		// 	for j, mg := range order.Group {
		// 		if m.WithOrder[i].Group[j].ModuleInfo, err = processModuleInfo(mg.ModuleInfo, m.relativeBaseDir, &target, getIndexManifest); err != nil {
		// 			return err
		// 		}
		// 	}
		// }

		// distro := dist.Distribution{}
		// if len(target.Distributions) != 0 {
		// 	distro = target.Distributions[0]
		// }

		// version := ""
		// if len(distro.Versions) != 0 {
		// 	version = distro.Versions[0]
		// }
		// bpPath := filepath.Join(buildpack.PlatformRootDirectory(target, distro.Name, version), BuildpackToml)
		// path, err := filepath.Abs(filepath.Join(m.relativeBaseDir, bpPath))
		// if err != nil {
		// 	return err
		// }

		// if m.bpType != Composite {
		writeBPPath := m.Path()
		os.MkdirAll(filepath.Dir(writeBPPath), os.ModePerm)
		bpFile, err := os.Create(writeBPPath)
		if err != nil {
			return err
		}
		return toml.NewEncoder(bpFile).Encode(m.BuildpackDescriptor)
		// }

		// return nil
	}
	return errors.New("invalid MultiArchBuildpackConfig")
}

func (m *MultiArchBuildpackConfig) BuildpackType() BuildpackType {
	return m.bpType
}

func (m *MultiArchBuildpackConfig) RelativeBaseDir() string {
	return m.relativeBaseDir
}

// func processModuleInfo(module dist.ModuleInfo, relativeBaseDir string, target *dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (m dist.ModuleInfo, err error) {
// 	if module.ID == "" {
// 		return module, errors.New("module id must be defined")
// 	}

// 	if module.Version == "" {
// 		module.Version = "latest"
// 	}

// 	module.ID, err = getRelativeURI(module.ID, relativeBaseDir, target, getIndexManifest)
// 	return module, err
// }

func (m *MultiArchBuildpackConfig) CleanBuildpackToml() error {
	return os.Remove(m.Path())
}

func (m *MultiArchPackage) RelativeBaseDir() string {
	return m.relativeBaseDir
}

func (m *MultiArchPackage) CopyPackageToml(relativeTo string, target dist.Target, distroName, version string, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (err error) {
	if m.Buildpack.URI != "" || m.Extension.URI != "" {
		if uri := m.Buildpack.URI; uri != "" {
			if m.Buildpack.URI, err = getRelativeURI(uri, m.relativeBaseDir, &target, getIndexManifest); err != nil {
				return err
			}
		}

		if uri := m.Extension.URI; uri != "" {
			if m.Extension.URI, err = getRelativeURI(uri, m.relativeBaseDir, &target, getIndexManifest); err != nil {
				return err
			}
		}

		for i, dep := range m.Dependencies {
			// dep.ImageName == dep.ImageRef.ImageName, dep.URI == dep.Buildpack.URI
			if dep.URI != "" {
				if m.Dependencies[i].URI, err = getRelativeURI(dep.URI, m.relativeBaseDir, &target, getIndexManifest); err != nil {
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
		return toml.NewEncoder(bpFile).Encode(m.Config)
	}
	return nil
}

func getRelativeURI(uri, relativeBaseDir string, target *dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (string, error) {
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
		return paths.FilePathToURI(filepath.Join(uri, buildpack.PlatformRootDirectory(target, distro.Name, version)), relativeBaseDir)
	case buildpack.PackageLocator:
		if target == nil {
			return "", fmt.Errorf("nil target")
		}
		ref, err := parseURItoString(buildpack.ParsePackageLocator(uri), *target, getIndexManifest)
		return "docker://" + ref, err
	case buildpack.RegistryLocator:
		if target == nil {
			return "", fmt.Errorf("nil target")
		}

		rNS, rName, rVersion, err := buildpack.ParseRegistryID(uri)
		if err != nil {
			return uri, err
		}

		ref, err := parseURItoString(rNS+RegistryDelim+rName+DigestDelim+rVersion, *target, getIndexManifest)
		return "urn:cnb:registry:" + ref, err
	case buildpack.FromBuilderLocator:
		return uri, fmt.Errorf("buildpacks doesn't support '%s' or '%s'", "from=builder", "urn:cnb:builder")
	case buildpack.IDLocator:
		return uri, fmt.Errorf("buildpacks doesn't support '%s'", "IDLocator")
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

func parseURItoString(uri string, target dist.Target, getIndexManifest func(ref name.Reference) (*v1.IndexManifest, error)) (string, error) {
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

	digest, err := DigestFromIndex(idx, target)
	if err != nil {
		return "", err
	}

	return ref.Context().Name() + DigestDelim + digest, nil
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

	for _, mfest := range idx.Manifests {
		if mfest.Platform == nil {
			return "", imgutil.ErrPlatformUndefined
		}

		platform := mfest.Platform
		if platform.Satisfies(*target.Platform()) {
			return mfest.Digest.String(), nil
		}
	}

	return "", errors.New("no image found with given platform")
}
