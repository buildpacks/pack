package lifecycle

import (
	"log"

	"github.com/buildpack/lifecycle/image"
)

type Analyzer struct {
	Buildpacks []*Buildpack
	AppDir     string
	LayersDir  string
	In         []byte
	Out, Err   *log.Logger
}

func (a *Analyzer) Analyze(image image.Image) error {
	metadata, err := getAppMetadata(image, a.Out)
	if err != nil {
		return err
	}
	for _, buildpack := range a.Buildpacks {
		cache, err := readBuildpackLayersDir(a.LayersDir, *buildpack)
		if err != nil {
			return err
		}

		metadataLayers := metadata.metadataForBuildpack(buildpack.ID).Layers
		for _, cachedLayer := range cache.layers {
			cacheType := cachedLayer.classifyCache(metadataLayers)
			switch cacheType {
			case cacheStaleNoMetadata:
				a.Out.Printf("removing stale cached launch layer '%s', not in metadata \n", cachedLayer.Identifier())
				if err := cachedLayer.remove(); err != nil {
					return err
				}
			case cacheStaleWrongSHA:
				a.Out.Printf("removing stale cached launch layer '%s'", cachedLayer.Identifier())
				if err := cachedLayer.remove(); err != nil {
					return err
				}
			case cacheMalformed:
				a.Out.Printf("removing malformed cached layer '%s'", cachedLayer.Identifier())
				if err := cachedLayer.remove(); err != nil {
					return err
				}
			case cacheNotForLaunch:
				a.Out.Printf("using cached layer '%s'", cachedLayer.Identifier())
			case cacheValid:
				a.Out.Printf("using cached launch layer '%s'", cachedLayer.Identifier())
				a.Out.Printf("rewriting metadata for layer '%s'", cachedLayer.Identifier())
				if err := cachedLayer.writeMetadata(metadataLayers); err != nil {
					return err
				}
			}
		}

		for lmd, data := range metadataLayers {
			if !data.Build && !data.Cache {
				layer := cache.newBPLayer(lmd)
				a.Out.Printf("writing metadata for uncached layer '%s'", layer.Identifier())
				if err := layer.writeMetadata(metadataLayers); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
