package stack

import "github.com/google/go-containerregistry/pkg/name"

type Metadata struct {
	RunImage RunImageMetadata `toml:"run-image" json:"runImage"`
}

type RunImageMetadata struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors"`
}

func (m Metadata) GetBestMirror(repoName string, localMirrors []string) (string, error) {
	desiredRegistry, err := registry(repoName)
	if err != nil {
		return "", err
	}

	runImageList := append(localMirrors, append([]string{m.RunImage.Image}, m.RunImage.Mirrors...)...)
	for _, img := range runImageList {
		if reg, err := registry(img); err == nil && reg == desiredRegistry {
			return img, nil
		}
	}

	if len(localMirrors) > 0 {
		return localMirrors[0], nil
	}

	return m.RunImage.Image, nil
}

func registry(imageName string) (string, error) {
	ref, err := name.ParseReference(imageName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}
