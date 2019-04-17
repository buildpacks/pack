package metadata

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/image"
)

const AppMetadataLabel = "io.buildpacks.lifecycle.metadata"

type AppImageMetadata struct {
	App        AppMetadata         `json:"app"`
	Config     ConfigMetadata      `json:"config"`
	Launcher   LauncherMetadata    `json:"launcher"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	RunImage   RunImageMetadata    `json:"runImage"`
	Stack      StackMetadata       `json:"stack"`
}

type AppMetadata struct {
	SHA string `json:"sha"`
}

type ConfigMetadata struct {
	SHA string `json:"sha"`
}

type LauncherMetadata struct {
	SHA string `json:"sha"`
}

type BuildpackMetadata struct {
	ID      string                   `json:"key"`
	Version string                   `json:"version"`
	Layers  map[string]LayerMetadata `json:"layers"`
}

type LayerMetadata struct {
	SHA    string      `json:"sha" toml:"-"`
	Data   interface{} `json:"data" toml:"metadata"`
	Build  bool        `json:"build" toml:"build"`
	Launch bool        `json:"launch" toml:"launch"`
	Cache  bool        `json:"cache" toml:"cache"`
}

type RunImageMetadata struct {
	TopLayer string `json:"topLayer"`
	SHA      string `json:"sha"`
}

type StackMetadata struct {
	RunImage StackRunImageMetadata `toml:"run-image" json:"runImage"`
}

type StackRunImageMetadata struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors,omitempty"`
}

func (m *AppImageMetadata) MetadataForBuildpack(id string) BuildpackMetadata {
	for _, bpMd := range m.Buildpacks {
		if bpMd.ID == id {
			return bpMd
		}
	}
	return BuildpackMetadata{}
}

func GetAppMetadata(image image.Image) (AppImageMetadata, error) {
	contents, err := GetRawMetadata(image, AppMetadataLabel)
	if err != nil {
		return AppImageMetadata{}, err
	}

	meta := AppImageMetadata{}
	_ = json.Unmarshal([]byte(contents), &meta)
	return meta, nil
}

func GetRawMetadata(image image.Image, metadataLabel string) (string, error) {
	if found, err := image.Found(); err != nil {
		return "", err
	} else if !found {
		return "", nil
	}
	contents, err := image.Label(metadataLabel)
	if err != nil {
		return "", errors.Wrapf(err, "retrieving label '%s' for image '%s'", metadataLabel, image.Name())
	}
	return contents, nil
}
