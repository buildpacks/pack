package buildpack

import (
	"github.com/pkg/errors"
	"strings"
)

// ParseIDLocator parses a buildpack locator of the form <id>@<version> into its ID and version.
// If version is omitted, the version returned will be empty. Any "from=builder:" prefix will be ignored.
func ParseIDLocator(locator string) (id string, version string) {
	parts := strings.Split(strings.TrimPrefix(locator, fromBuilderPrefix+":"), "@")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func ParseRegistryID(registryID string) (namespace string, name string, version string, err error) {
	id, version := ParseIDLocator(registryID)

	parts := strings.Split(id, "/")
	if len(parts) == 2 {
		return parts[0], parts[1], version, nil
	}
	return parts[0], "", version, errors.Errorf("invalid registry ID: %s", registryID)
}