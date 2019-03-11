package lifecycle

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/image"
)

type Cacher struct {
	ArtifactsDir string
	Buildpacks   []*Buildpack
	Out, Err     *log.Logger
	UID, GID     int
}

func (c *Cacher) Cache(layersDir string, oldCacheImage, newCacheImage image.Image) error {
	loggingCacheImage := &loggingImage{
		Out:   c.Out,
		image: newCacheImage,
	}

	origMetadata, err := getCacheMetadata(oldCacheImage, c.Out)
	if err != nil {
		return errors.Wrap(err, "metadata for previous image")
	}

	newMetadata := CacheImageMetadata{
		Buildpacks: []BuildpackMetadata{},
	}

	for _, bp := range c.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, *bp)
		if err != nil {
			return err
		}
		bpMetadata := BuildpackMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]LayerMetadata{},
		}
		for _, l := range bpDir.findLayers(cached) {
			if !l.hasLocalContents() {
				return fmt.Errorf("failed to cache layer '%s' because it has no contents", l.Identifier())
			}
			metadata, err := l.read()
			if err != nil {
				return err
			}
			origLayerMetadata := origMetadata.metadataForBuildpack(bp.ID).Layers[l.name()]
			if metadata.SHA, err = c.addOrReuseLayer(loggingCacheImage, l, origLayerMetadata.SHA); err != nil {
				return err
			}
			bpMetadata.Layers[l.name()] = metadata
		}
		newMetadata.Buildpacks = append(newMetadata.Buildpacks, bpMetadata)
	}
	data, err := json.Marshal(newMetadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	if err := loggingCacheImage.SetLabel(CacheMetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	sha, err := loggingCacheImage.Save()
	if err == nil {
		c.Out.Printf("cache '%s@%s'", newCacheImage.Name(), sha)
	}
	return err
}

func (c *Cacher) addOrReuseLayer(image *loggingImage, layer bpLayer, previousSHA string) (string, error) {
	tarPath := filepath.Join(c.ArtifactsDir, escapeIdentifier(layer.Identifier())+".tar")
	sha, err := archive.WriteTarFile(layer.Path(), tarPath, c.UID, c.GID)
	if err != nil {
		return "", errors.Wrapf(err, "caching layer '%s'", layer.Identifier())
	}
	if sha == previousSHA {
		return sha, image.ReuseLayer(layer.Identifier(), previousSHA)
	}
	return sha, image.AddLayer(layer.Identifier(), sha, tarPath)
}
