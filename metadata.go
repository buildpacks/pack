package pack

import (
	"github.com/google/go-containerregistry/pkg/name"
)

const (
	BuilderMetadataLabel = "io.buildpacks.builder.metadata"
	RunImageLabel        = "io.buildpacks.run-image"
)

type BuilderImageMetadata struct {
	RunImage   BuilderRunImageMetadata     `json:"runImage"`
	Buildpacks []BuilderBuildpacksMetadata `json:"buildpacks"`
}

type BuilderRunImageMetadata struct {
	Image   string   `json:"image"`
	Mirrors []string `json:"mirrors"`
}

type BuilderBuildpacksMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func (m *BuilderImageMetadata) RunImageForRepoName(repoName string, runImages []string) (runImage string, locallyConfigured bool, err error) {
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
