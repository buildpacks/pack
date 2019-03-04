package lifecycle

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

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
	found, err := image.Found()
	if err != nil {
		return err
	}

	var metadata AppImageMetadata
	if !found {
		a.Out.Printf("WARNING: image '%s' not found or requires authentication to access\n", image.Name())
	} else {
		metadata, err = a.getMetadata(image)
		if err != nil {
			return err
		}
	}
	return a.analyze(metadata)
}

func (a *Analyzer) analyze(metadata AppImageMetadata) error {
	groupBPs := a.buildpacks()

	err := a.removeOldBackpackLayersNotInGroup(groupBPs)
	if err != nil {
		return err
	}

	for buildpackID := range groupBPs {
		cache, err := readBuildpackLayersDir(a.LayersDir, buildpackID)
		if err != nil {
			return err
		}

		metadataLayers := a.layersToAnalyze(buildpackID, metadata)
		for _, cachedLayer := range cache.layers {
			cacheType := cachedLayer.classifyCache(metadataLayers)
			switch cacheType {
			case cacheStaleNoMetadata:
				a.Out.Printf("removing stale cached launch layer '%s:%s', not in metadata \n", buildpackID, cachedLayer)
				if err := cachedLayer.remove(); err != nil {
					return err
				}
			case cacheStaleWrongSHA:
				a.Out.Printf("removing stale cached launch layer '%s:%s'", buildpackID, cachedLayer)
				if err := cachedLayer.remove(); err != nil {
					return err
				}
			case cacheMalformed:
				a.Out.Printf("removing malformed cached layer '%s:%s'", buildpackID, cachedLayer)
				if err := cachedLayer.remove(); err != nil {
					return err
				}
			case cacheNotForLaunch:
				a.Out.Printf("using cached layer '%s/%s'", buildpackID, cachedLayer)
			case cacheValid:
				a.Out.Printf("using cached launch layer '%s:%s'", buildpackID, cachedLayer)
				a.Out.Printf("rewriting metadata for layer '%s:%s'", buildpackID, cachedLayer)
				if err := cachedLayer.writeMetadata(metadataLayers); err != nil {
					return err
				}
				delete(metadataLayers, cachedLayer.name())
			}
		}

		for layer, data := range metadataLayers {
			if !data.Build {
				a.Out.Printf("writing metadata for uncached layer '%s/%s'", buildpackID, layer)
				if err := cache.newBPLayer(layer).writeMetadata(metadataLayers); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (a *Analyzer) layersToAnalyze(buildpackID string, metadata AppImageMetadata) map[string]LayerMetadata {
	layers := make(map[string]LayerMetadata)
	for _, bp := range metadata.Buildpacks {
		if bp.ID == buildpackID {
			return bp.Layers
		}
	}
	return layers
}

func (a *Analyzer) getMetadata(image image.Image) (AppImageMetadata, error) {
	metadata := AppImageMetadata{}
	label, err := image.Label(MetadataLabel)
	if err != nil {
		return metadata, err
	}
	if label == "" {
		a.Out.Printf("WARNING: previous image '%s' does not have '%s' label", image.Name(), MetadataLabel)
		return metadata, nil
	}

	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		a.Out.Printf("WARNING: previous image '%s' has incompatible '%s' label\n", image.Name(), MetadataLabel)
		return metadata, nil
	}
	return metadata, nil
}

func (a *Analyzer) buildpacks() map[string]struct{} {
	buildpacks := make(map[string]struct{}, len(a.Buildpacks))
	for _, b := range a.Buildpacks {
		buildpacks[b.ID] = struct{}{}
	}
	return buildpacks
}

func (a *Analyzer) removeOldBackpackLayersNotInGroup(groupBPs map[string]struct{}) error {
	cachedBPs, err := a.cachedBuildpacks()
	if err != nil {
		return err
	}

	for _, cachedBP := range cachedBPs {
		_, exists := groupBPs[cachedBP]
		if !exists {
			a.Out.Printf("removing cached layers for buildpack '%s' not in group\n", cachedBP)
			if err := os.RemoveAll(filepath.Join(a.LayersDir, cachedBP)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (a *Analyzer) cachedBuildpacks() ([]string, error) {
	cachedBps := make([]string, 0, 0)
	bpDirs, err := filepath.Glob(filepath.Join(a.LayersDir, "*"))
	if err != nil {
		return nil, err
	}
	appDirInfo, err := os.Stat(a.AppDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "stat app dir")
	}
	for _, dir := range bpDirs {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !os.SameFile(appDirInfo, info) && info.IsDir() {
			cachedBps = append(cachedBps, buildpackDirToID(filepath.Base(dir)))
		}
	}
	return cachedBps, nil
}
