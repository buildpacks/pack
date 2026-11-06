package buildpack

import (
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/buildpacks/pack/pkg/dist"
)

var errLayoutPlatformNotFound = errors.New("no matching platform in package")

// findLayoutImage walks OCI indexes as well as direct image descriptors. The
// target may be nil for callers inspecting platform-independent package metadata.
func findLayoutImage(blob Blob, index v1.Index, target *dist.Target, depth int) (*v1.Manifest, *v1.Image, error) {
	if depth > 32 {
		return nil, nil, errors.New("package index nesting exceeds 32 levels")
	}
	for _, descriptor := range index.Manifests {
		if target != nil && descriptor.Platform != nil && !layoutTargetMatches(dist.Target{
			OS: descriptor.Platform.OS, Arch: descriptor.Platform.Architecture, ArchVariant: descriptor.Platform.Variant,
		}, *target) {
			continue
		}
		switch descriptor.MediaType {
		case v1.MediaTypeImageIndex, "application/vnd.docker.distribution.manifest.list.v2+json":
			var child v1.Index
			if err := unmarshalJSONFromBlob(blob, pathFromDescriptor(descriptor), &child); err != nil {
				return nil, nil, fmt.Errorf("reading package index %s: %w", descriptor.Digest, err)
			}
			manifest, config, err := findLayoutImage(blob, child, target, depth+1)
			if errors.Is(err, errLayoutPlatformNotFound) {
				continue
			}
			return manifest, config, err
		case v1.MediaTypeImageManifest, "application/vnd.docker.distribution.manifest.v2+json":
			var manifest v1.Manifest
			if err := unmarshalJSONFromBlob(blob, pathFromDescriptor(descriptor), &manifest); err != nil {
				return nil, nil, fmt.Errorf("reading package manifest %s: %w", descriptor.Digest, err)
			}
			var config v1.Image
			if err := unmarshalJSONFromBlob(blob, pathFromDescriptor(manifest.Config), &config); err != nil {
				return nil, nil, fmt.Errorf("reading package config %s: %w", manifest.Config.Digest, err)
			}
			if target != nil {
				if !layoutTargetMatches(dist.Target{OS: config.OS, Arch: config.Architecture, ArchVariant: config.Variant}, *target) {
					continue
				}
				matches, err := layoutDistributionsMatch(config.Config.Labels, *target)
				if err != nil {
					return nil, nil, err
				}
				if !matches {
					continue
				}
			}
			return &manifest, &config, nil
		}
	}
	return nil, nil, errLayoutPlatformNotFound
}

func layoutTargetMatches(available, requested dist.Target) bool {
	// Older packages can omit architecture information. Such a package remains
	// usable for targets it does not explicitly restrict.
	return (available.OS == "" || requested.OS == "" || available.OS == requested.OS) &&
		(available.Arch == "" || requested.Arch == "" || available.Arch == requested.Arch) &&
		(available.ArchVariant == "" || requested.ArchVariant == "" || available.ArchVariant == requested.ArchVariant)
}

func layoutDistributionsMatch(labels map[string]string, target dist.Target) (bool, error) {
	if len(target.Distributions) == 0 {
		return true, nil
	}
	var metadata Metadata
	if err := json.Unmarshal([]byte(labels[MetadataLabel]), &metadata); err != nil {
		return false, fmt.Errorf("reading package metadata: %w", err)
	}
	layersLabel := labels[dist.BuildpackLayersLabel]
	if layersLabel == "" {
		layersLabel = labels[dist.ExtensionLayersLabel]
	}
	var layers dist.ModuleLayers
	if err := json.Unmarshal([]byte(layersLabel), &layers); err != nil {
		return false, fmt.Errorf("reading package layer metadata: %w", err)
	}
	mainLayer, ok := layers.Get(metadata.ID, metadata.Version)
	if !ok {
		return false, errors.New("main module is missing from package layer metadata")
	}
	if len(mainLayer.Targets) == 0 {
		return true, nil
	}
	for _, supported := range mainLayer.Targets {
		if !layoutTargetMatches(supported, target) {
			continue
		}
		if len(supported.Distributions) == 0 {
			return true, nil
		}
		for _, available := range supported.Distributions {
			for _, requested := range target.Distributions {
				if (available.Name == "" || requested.Name == "" || available.Name == requested.Name) &&
					(available.Version == "" || requested.Version == "" || available.Version == requested.Version) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
