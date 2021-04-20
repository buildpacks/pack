package asset

import (
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/internal/paths"
)

// LocatorType represents the different locations an image may be retrieved from
type LocatorType int

const (
	// InvalidLocator represents an unknown or un-parsable locator type
	InvalidLocator LocatorType = iota
	// URI locator paths represent a file://, http:// or https:// URI
	URILocator
	// FilepathLocator represents a local file path (this may be relative)
	FilepathLocator
	// ImageLocator represents a valid image that may be expanded to <org>/<repo>:<tag>
	ImageLocator
)

// String represents each locator as a printable string
func (l LocatorType) String() string {
	return []string{
		"InvalidLocator",
		"URILocator",
		"FilepathLocator",
		"ImageLocator",
	}[l]
}

// GetLocatorType parses a locator and returns the LocatorType
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
