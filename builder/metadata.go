package builder

import (
	"github.com/buildpack/lifecycle"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpack/pack/buildpack"
)

const MetadataLabel = "io.buildpacks.builder.metadata"

type TOML struct {
	Buildpacks []buildpack.Buildpack      `toml:"buildpacks"`
	Groups     []lifecycle.BuildpackGroup `toml:"groups"`
	Stack      Stack
}

type Stack struct {
	ID              string   `toml:"id"`
	BuildImage      string   `toml:"build-image"`
	RunImage        string   `toml:"run-image"`
	RunImageMirrors []string `toml:"run-image-mirrors,omitempty"`
}

type Metadata struct {
	RunImage   RunImageMetadata    `json:"runImage"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	Groups     []GroupMetadata     `json:"groups"`
}

type RunImageMetadata struct {
	Image   string   `json:"image"`
	Mirrors []string `json:"mirrors"`
}

type BuildpackMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Latest  bool   `json:"latest"`
}

type GroupMetadata struct {
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
}

func (m *Metadata) RunImageForRepoName(repoName string, runImages []string) (runImage string, locallyConfigured bool, err error) {
	desiredRegistry, err := registry(repoName)
	if err != nil {
		return "", false, err
	}

	for _, image := range runImages {
		if reg, err := registry(image); err == nil && reg == desiredRegistry {
			return image, true, nil
		}
	}

	for _, image := range append([]string{m.RunImage.Image}, m.RunImage.Mirrors...) {
		if reg, err := registry(image); err == nil && reg == desiredRegistry {
			return image, false, nil
		}
	}

	//todo like uh should we do this
	if len(runImages) > 0 {
		return runImages[0], true, nil
	}

	return m.RunImage.Image, false, nil
}

func registry(imageName string) (string, error) {
	ref, err := name.ParseReference(imageName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}
