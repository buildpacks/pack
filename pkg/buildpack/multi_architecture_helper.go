package buildpack

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
	"github.com/buildpacks/pack/pkg/dist"
	"github.com/buildpacks/pack/pkg/logging"
)

type MultiArchConfig struct {
	buildpackTargets []dist.Target
	expectedTargets  []dist.Target
	logger           logging.Logger
}

func NewMultiArchConfig(targets []dist.Target, expected []dist.Target, logger logging.Logger) (MultiArchConfig, error) {
	// Let's do some validations
	return MultiArchConfig{
		buildpackTargets: targets,
		expectedTargets:  expected,
		logger:           logger,
	}, nil
}

func (m *MultiArchConfig) Targets() []dist.Target {
	if len(m.expectedTargets) == 0 {
		return m.buildpackTargets
	}
	return m.expectedTargets
}

// CopyConfigFiles Given a base directory, which is expected to be the root folder of single buildpack,
// it will copy a buildpack.toml file, for each target, into the corresponding platform root folder.
func (m *MultiArchConfig) CopyConfigFiles(baseDir string) ([]string, error) {
	var filesToClean []string
	for _, target := range m.Targets() {
		// TODO we are not handling distributions versions yet
		path, err := CopyConfigFile(baseDir, target, "")
		if err != nil {
			return nil, err
		}
		if path != "" {
			filesToClean = append(filesToClean, path)
		}
	}
	return filesToClean, nil
}

// PrepareDependencyConfigFile when creating a composite buildpack, dependencies URI are relative to the main buildpack
// if we are building a multi-arch composite buildpack we MAY need to copy the buildpack.toml file to the dependency and
// determine the platform root folder. This method will do those operations and return the path to the buildpack.toml that
// was copied.
func PrepareDependencyConfigFile(baseDir, depURI string, target dist.Target, version string, failWhenURI bool) (string, error) {
	// Only in cases it is a URILocator we may want to copy config files
	locatorType, err := GetLocatorType(depURI, baseDir, []dist.ModuleInfo{})
	if err != nil {
		return "", err
	}

	if locatorType == URILocator {
		if failWhenURI {
			return "", errors.New(fmt.Sprintf("%s is not allowed when creating a composite buildpack, use 'docker://' instead", style.Symbol(depURI)))
		}

		uri, err := paths.FilePathToURI(depURI, baseDir)
		if err != nil {
			return "", errors.Wrapf(err, "making absolute: %s", style.Symbol(depURI))
		}

		if paths.IsURI(uri) {
			parsedURL, err := url.Parse(uri)
			if err != nil {
				return "", errors.Wrapf(err, "parsing path/uri %s", style.Symbol(uri))
			}

			if parsedURL.Scheme == "file" {
				path, err := paths.URIToFilePath(uri)
				if err != nil {
					return "", err
				}
				if exists, _ := paths.IsDir(path); exists {
					return CopyConfigFile(path, target, version)
				}
			}
		}
	}
	return "", nil
}

// CopyConfigFile will determine and copy the buildpack.toml file into the corresponding platform folder
func CopyConfigFile(baseDir string, target dist.Target, version string) (string, error) {
	if ok, platformRootFolder := PlatformRootFolder(baseDir, target, version); ok {
		path, err := copy(baseDir, platformRootFolder)
		if err != nil {
			return "", err
		}
		return path, nil
	}
	return "", nil
}

// PlatformRootFolder calculates the top-most directory that identifies a target in a given <root> folder.
// Let's define a target with the following format: [os][/arch][/variant]:[name@version], consider the following examples:
//   - Given a target linux/amd64 the platform root folder will be <root>/linux/amd64 if the folder exists
//   - Given a target windows/amd64:windows@10.0.20348.1970 the platform root folder will be <root>/windows/amd64/windows@10.0.20348.1970 if the folder exists
//   - When no target folder exists, the root folder will be equal to <root> folder
func PlatformRootFolder(root string, target dist.Target, version string) (bool, string) {
	targets := target.ValuesAsSlide(version)
	pRootFolder := root

	found := false
	current := false
	for _, t := range targets {
		current, pRootFolder = targetExists(pRootFolder, t)
		if current {
			found = current
		} else {
			// Not need to keep looking deeply
			break
		}
	}
	// We will return the last matching folder
	return found, pRootFolder
}

func targetExists(root, expected string) (bool, string) {
	if expected == "" {
		return false, root
	}
	path := filepath.Join(root, expected)
	if exists, _ := paths.IsDir(path); exists {
		return true, path
	}
	return false, root
}

func copy(src string, dest string) (string, error) {
	filePath := filepath.Join(dest, "buildpack.toml")
	fileToCopy, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer fileToCopy.Close()

	builpackTomlFile, err := os.Open(filepath.Join(src, "buildpack.toml"))
	if err != nil {
		return "", err
	}
	defer builpackTomlFile.Close()

	_, err = io.Copy(fileToCopy, builpackTomlFile)
	if err != nil {
		return "", err
	}

	fileToCopy.Sync()
	if err != nil {
		return "", err
	}

	return filePath, nil
}
