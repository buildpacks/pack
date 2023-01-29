package inspectimage

import (
	"github.com/buildpacks/lifecycle/buildpack"

	"github.com/buildpacks/pack/pkg/client"
	"github.com/buildpacks/pack/pkg/dist"
)

type BOMDisplay struct {
	Remote    []BOMEntryDisplay `json:"remote" yaml:"remote"`
	Local     []BOMEntryDisplay `json:"local" yaml:"local"`
	RemoteErr string            `json:"remote_error,omitempty" yaml:"remoteError,omitempty"`
	LocalErr  string            `json:"local_error,omitempty" yaml:"localError,omitempty"`
}

type BOMWithExtensionDisplay struct {
	Remote    []BOMEntryWithExtensionDisplay `json:"remote" yaml:"remote"`
	Local     []BOMEntryWithExtensionDisplay `json:"local" yaml:"local"`
	RemoteErr string                         `json:"remote_error,omitempty" yaml:"remoteError,omitempty"`
	LocalErr  string                         `json:"local_error,omitempty" yaml:"localError,omitempty"`
}

type BOMEntryDisplay struct {
	Name      string                 `toml:"name" json:"name" yaml:"name"`
	Version   string                 `toml:"version,omitempty" json:"version,omitempty" yaml:"version,omitempty"`
	Metadata  map[string]interface{} `toml:"metadata" json:"metadata" yaml:"metadata"`
	Buildpack dist.ModuleRef         `json:"buildpacks" yaml:"buildpacks" toml:"buildpacks"`
}

type BOMEntryWithExtensionDisplay struct {
	Name      string                 `toml:"name" json:"name" yaml:"name"`
	Version   string                 `toml:"version,omitempty" json:"version,omitempty" yaml:"version,omitempty"`
	Metadata  map[string]interface{} `toml:"metadata" json:"metadata" yaml:"metadata"`
	Buildpack dist.ModuleRef         `json:"buildpacks" yaml:"buildpacks" toml:"buildpacks"`
	Extension dist.ModuleRef         `json:"extensions" yaml:"extensions" toml:"extensions"`
}

func NewBOMDisplay(info *client.ImageInfo) []BOMEntryDisplay {
	if info == nil {
		return nil
	}
	return displayBOM(info.BOM)
}

func NewBOMWithExtensionDisplay(infoWithExtension *client.ImageWithExtensionInfo) []BOMEntryWithExtensionDisplay {
	if infoWithExtension == nil {
		return nil
	}
	return displayBOMWithExtension(infoWithExtension.BOM)
}

func displayBOM(bom []buildpack.BOMEntry) []BOMEntryDisplay {
	var result []BOMEntryDisplay
	for _, entry := range bom {
		result = append(result, BOMEntryDisplay{
			Name:     entry.Name,
			Version:  entry.Version,
			Metadata: entry.Metadata,

			Buildpack: dist.ModuleRef{
				ModuleInfo: dist.ModuleInfo{
					ID:      entry.Buildpack.ID,
					Version: entry.Buildpack.Version,
				},
				Optional: entry.Buildpack.Optional,
			},
		})
	}

	return result
}

func displayBOMWithExtension(bom []buildpack.BOMEntry) []BOMEntryWithExtensionDisplay {
	var result []BOMEntryWithExtensionDisplay
	for _, entry := range bom {
		result = append(result, BOMEntryWithExtensionDisplay{
			Name:     entry.Name,
			Version:  entry.Version,
			Metadata: entry.Metadata,

			Buildpack: dist.ModuleRef{
				ModuleInfo: dist.ModuleInfo{
					ID:      entry.Buildpack.ID,
					Version: entry.Buildpack.Version,
				},
				Optional: entry.Buildpack.Optional,
			},
			Extension: dist.ModuleRef{
				ModuleInfo: dist.ModuleInfo{
					ID:      entry.Buildpack.ID,
					Version: entry.Buildpack.Version,
				},
				Optional: entry.Buildpack.Optional,
			},
		})
	}

	return result
}
