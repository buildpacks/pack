package lifecycle

import (
	"encoding/json"
	"log"

	"github.com/buildpack/lifecycle/image"
)

const (
	MetadataLabel      = "io.buildpacks.lifecycle.metadata"
	CacheMetadataLabel = "io.buildpacks.lifecycle.cache.metadata"
)

type AppImageMetadata struct {
	App        AppMetadata         `json:"app"`
	Config     ConfigMetadata      `json:"config"`
	Launcher   LauncherMetadata    `json:"launcher"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	RunImage   RunImageMetadata    `json:"runImage"`
}

type CacheImageMetadata struct {
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
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

func (m *AppImageMetadata) metadataForBuildpack(id string) BuildpackMetadata {
	for _, bpMd := range m.Buildpacks {
		if bpMd.ID == id {
			return bpMd
		}
	}
	return BuildpackMetadata{}
}

func (m *CacheImageMetadata) metadataForBuildpack(id string) BuildpackMetadata {
	for _, bpMd := range m.Buildpacks {
		if bpMd.ID == id {
			return bpMd
		}
	}
	return BuildpackMetadata{}
}

func getAppMetadata(image image.Image, log *log.Logger) (AppImageMetadata, error) {
	metadata := AppImageMetadata{}
	contents, err := getMetadata(image, MetadataLabel, log)
	if err != nil {
		return metadata, err
	}

	if err := json.Unmarshal([]byte(contents), &metadata); err != nil {
		log.Printf("WARNING: image '%s' has incompatible '%s' label\n", image.Name(), MetadataLabel)
		return AppImageMetadata{}, nil
	}
	return metadata, nil
}

func getCacheMetadata(image image.Image, log *log.Logger) (CacheImageMetadata, error) {
	metadata := CacheImageMetadata{}
	contents, err := getMetadata(image, CacheMetadataLabel, log)
	if err != nil {
		return metadata, err
	}

	if err := json.Unmarshal([]byte(contents), &metadata); err != nil {
		log.Printf("WARNING: image '%s' has incompatible '%s' label\n", image.Name(), CacheMetadataLabel)
		return CacheImageMetadata{}, nil
	}
	return metadata, nil
}

func getMetadata(image image.Image, metadataLabel string, log *log.Logger) (string, error) {
	if found, err := image.Found(); err != nil {
		return "", err
	} else if !found {
		log.Printf("WARNING: image '%s' not found or requires authentication to access\n", image.Name())
		return "", nil
	}
	contents, err := image.Label(metadataLabel)
	if err != nil {
		return "", err
	}
	if contents == "" {
		log.Printf("WARNING: image '%s' does not have '%s' label", image.Name(), metadataLabel)
		return "", nil
	}
	return contents, nil
}
