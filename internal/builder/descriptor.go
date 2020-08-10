package builder

import (
	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/api"
	"github.com/pkg/errors"
)

// LifecycleDescriptor contains information described in the lifecycle.toml
type LifecycleDescriptor struct {
	Info LifecycleInfo `toml:"lifecycle"`
	// Deprecated: Use `LifecycleAPIs` instead
	API  LifecycleAPI  `toml:"api"`
	APIs LifecycleAPIs `toml:"apis"`
}

// LifecycleInfo contains information about the lifecycle
type LifecycleInfo struct {
	Version *Version `toml:"version" json:"version"`
}

// LifecycleAPI describes which API versions the lifecycle satisfies
type LifecycleAPI struct {
	BuildpackVersion *api.Version `toml:"buildpack" json:"buildpack"`
	PlatformVersion  *api.Version `toml:"platform" json:"platform"`
}

// LifecycleAPIs describes the supported API versions per specification
type LifecycleAPIs struct {
	Buildpack APIVersions `toml:"buildpack" json:"buildpack"`
	Platform  APIVersions `toml:"platform" json:"platform"`
}

type APISet []*api.Version

func (a APISet) AsStrings() []string {
	verStrings := make([]string, len(a))
	for i, version := range a {
		verStrings[i] = version.String()
	}

	return verStrings
}

// APIVersions describes the supported API versions
type APIVersions struct {
	Deprecated APISet `toml:"deprecated" json:"deprecated"`
	Supported  APISet `toml:"supported" json:"supported"`
}

// ParseDescriptor parses LifecycleDescriptor from toml formatted string.
func ParseDescriptor(contents string) (LifecycleDescriptor, error) {
	descriptor := LifecycleDescriptor{}
	_, err := toml.Decode(contents, &descriptor)
	if err != nil {
		return descriptor, errors.Wrap(err, "decoding descriptor")
	}

	return descriptor, nil
}

// CompatDescriptor provides compatibility by mapping new fields to old and vice-versa
func CompatDescriptor(descriptor LifecycleDescriptor) LifecycleDescriptor {
	if len(descriptor.APIs.Buildpack.Supported) != 0 || len(descriptor.APIs.Platform.Supported) != 0 {
		// select earliest value for deprecated parameters
		if len(descriptor.APIs.Buildpack.Supported) != 0 {
			descriptor.API.BuildpackVersion = findEarliestVersion(
				append(descriptor.APIs.Buildpack.Deprecated, descriptor.APIs.Buildpack.Supported...),
			)
		}
		if len(descriptor.APIs.Platform.Supported) != 0 {
			descriptor.API.PlatformVersion = findEarliestVersion(
				append(descriptor.APIs.Platform.Deprecated, descriptor.APIs.Platform.Supported...),
			)
		}
	} else if descriptor.API.BuildpackVersion != nil && descriptor.API.PlatformVersion != nil {
		// fill supported with deprecated field
		descriptor.APIs = LifecycleAPIs{
			Buildpack: APIVersions{
				Supported: APISet{descriptor.API.BuildpackVersion},
			},
			Platform: APIVersions{
				Supported: APISet{descriptor.API.PlatformVersion},
			},
		}
	}

	return descriptor
}

func findEarliestVersion(versions []*api.Version) *api.Version {
	var earliest *api.Version
	for _, version := range versions {
		switch {
		case version == nil:
			continue
		case earliest == nil:
			earliest = version
		case version.Compare(earliest) < 0:
			earliest = version
		}
	}

	return earliest
}
