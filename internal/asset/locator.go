package asset

import (
	"github.com/buildpacks/pack/internal/paths"
	"github.com/google/go-containerregistry/pkg/name"
)

type LocatorType int

const (
	InvalidLocator LocatorType = iota
	URILocator
	FilepathLocator
	ImageLocator
)

func (l LocatorType) String() string {
	return []string{
		"InvalidLocator",
		"URILocator",
		"FilepathLocator",
		"ImageLocator",
	}[l]
}

func GetLocatorType(locator string, relativeBaseDir string) LocatorType {
	switch {
	case paths.IsURI(locator):
		return URILocator
	case isLocalFile(locator, relativeBaseDir):
		return FilepathLocator
	case canBeImageRef(locator):
		return ImageLocator
	default:
		return InvalidLocator
	}
}

func canBeImageRef(locator string) bool {
	if _, err := name.ParseReference(locator); err == nil {
		return true
	}

	return false
}

func isLocalFile(path, relativeBaseDir string) bool {
	_, result := localFile(path, relativeBaseDir)
	return result
}

