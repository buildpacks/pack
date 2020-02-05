package buildpack

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/pack/internal/paths"
	"github.com/buildpacks/pack/internal/style"
)

type LocatorType int

const (
	InvalidLocator = iota
	FromBuilderLocator
	URILocator
	IDLocator
	PackageLocator
)

const fromBuilderPrefix = "from=builder"

func (l LocatorType) String() string {
	return []string{
		"InvalidLocator",
		"FromBuilderLocator",
		"URILocator",
		"IDLocator",
		"PackageLocator",
	}[l]
}

// GetLocatorType determines which type of locator is designated by the given input.
// If a type cannot be determined, `INVALID_LOCATOR` will be returned. If an error
// is encountered, it will be returned.
func GetLocatorType(locator string, idsFromBuilder []string) (LocatorType, error) {
	if locator == fromBuilderPrefix {
		return FromBuilderLocator, nil
	}

	if strings.HasPrefix(locator, fromBuilderPrefix+":") {
		if !builderMatchFound(locator, idsFromBuilder) {
			return InvalidLocator, fmt.Errorf("%s is not a valid identifier", style.Symbol(locator))
		}
		return IDLocator, nil
	}

	if paths.IsURI(locator) {
		return URILocator, nil
	}

	exists, err := paths.Exists(locator)
	if err != nil {
		return InvalidLocator, err
	}
	if exists {
		return URILocator, nil
	}

	if builderMatchFound(locator, idsFromBuilder) {
		return IDLocator, nil
	}

	if _, err := name.ParseReference(locator); err == nil {
		return PackageLocator, nil
	}

	return InvalidLocator, nil
}

func builderMatchFound(locator string, candidates []string) bool {
	id, version := ParseIDLocator(locator)
	for _, c := range candidates {
		candID, candVer := ParseIDLocator(c)
		if id == candID && (version == "" || version == candVer) {
			return true
		}
	}
	return false
}
