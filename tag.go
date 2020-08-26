package pack

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

// Default Namespace used by docker
const defaultNamespace = "library"

// Tag stores a docker tag name with the following form:
//    <registry>/<namespace>/<name>:<tag>
type Tag struct {
	name.Tag
	dockerPrefixSpecified bool
}

// NewTag creates a new for the given tagName.
// This tag will be validated against strictness defined by the opts.
func NewTag(tagName string, opts ...name.Option) (Tag, error) {
	minPrefix := fmt.Sprintf("%s/", name.DefaultRegistry)
	tag, err := name.NewTag(tagName, opts...)
	if err != nil {
		return Tag{}, fmt.Errorf("error creating tag: %q", err)
	}
	return Tag{
		Tag:                   tag,
		dockerPrefixSpecified: strings.HasPrefix(tagName, minPrefix),
	}, nil
}

// Context accesses the Repository context of the reference.
func (pr Tag) Context() name.Repository {
	return pr.Tag.Context()
}

// Identifier accesses the type-specific portion of the reference.
// This corresponds to the <tag> section.
func (pr Tag) Identifier() string {
	return pr.Tag.Identifier()
}

// Name returns the fully-qualified reference name, this is the entire
// <registry>/<namespace>/<name>:<tag> string.
// If <registry> or <namespace> were omitted during the creation of this Tag, they will be missing from
// the string returned by this function.
func (pr Tag) Name() string {
	minPrefix := fmt.Sprintf("%s/", name.DefaultRegistry)
	maxPrefix := fmt.Sprintf("%s%s/", minPrefix, defaultNamespace)

	result := pr.Tag.Name()

	if pr.Registry.RegistryStr() == name.DefaultRegistry && !pr.dockerPrefixSpecified {
		result = strings.TrimPrefix(result, maxPrefix)
		result = strings.TrimPrefix(result, minPrefix)
	}
	return result
}

// Scope is the scope needed to access this reference.
func (pr Tag) Scope(action string) string {
	return pr.Tag.Scope(action)
}
