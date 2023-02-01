package client

import (
	"fmt"
	"sort"

	"github.com/buildpacks/pack/pkg/buildpack"
	"github.com/buildpacks/pack/pkg/dist"
)

type ExtensionInfo struct {
	ExtensionMetadata buildpack.Metadata
	Extensions        []dist.ModuleInfo
	Order             dist.Order
	ExtensionLayers   dist.ModuleLayers
	Location          buildpack.LocatorType
}

type InspectExtensionOptions struct {
	ExtensionName string
	Daemon        bool
	Registry      string
}

func (c *Client) InspectExtension(opts InspectExtensionOptions) (*ExtensionInfo, error) {
	locatorType, err := buildpack.GetLocatorType(opts.ExtensionName, "", []dist.ModuleInfo{})
	if err != nil {
		return nil, err
	}
	var layersMd dist.ModuleLayers
	var extensionMd buildpack.Metadata

	switch locatorType {
	case buildpack.RegistryLocator:
		extensionMd, layersMd, err = metadataFromRegistry(c, opts.ExtensionName, opts.Registry)
	case buildpack.PackageLocator:
		extensionMd, layersMd, err = metadataFromImage(c, opts.ExtensionName, opts.Daemon)
	case buildpack.URILocator:
		extensionMd, layersMd, err = metadataFromArchive(c.downloader, opts.ExtensionName)
	default:
		return nil, fmt.Errorf("unable to handle locator %q: for extension %q", locatorType, opts.ExtensionName)
	}
	if err != nil {
		return nil, err
	}

	return &ExtensionInfo{
		ExtensionMetadata: extensionMd,
		ExtensionLayers:   layersMd,
		Order:             extractOrder(extensionMd),
		Extensions:        extractExtension(layersMd),
		Location:          locatorType,
	}, nil
}

func extractExtension(layersMd dist.ModuleLayers) []dist.ModuleInfo {
	result := []dist.ModuleInfo{}
	extensionSet := map[*dist.ModuleInfo]bool{}

	for extensionID, extensionMap := range layersMd {
		for version, layerInfo := range extensionMap {
			ex := dist.ModuleInfo{
				ID:       extensionID,
				Name:     layerInfo.Name,
				Version:  version,
				Homepage: layerInfo.Homepage,
			}
			extensionSet[&ex] = true
		}
	}

	for currentExtension := range extensionSet {
		result = append(result, *currentExtension)
	}

	sort.Slice(result, func(i int, j int) bool {
		switch {
		case result[i].ID < result[j].ID:
			return true
		case result[i].ID == result[j].ID:
			return result[i].Version < result[j].Version
		default:
			return false
		}
	})
	return result
}
