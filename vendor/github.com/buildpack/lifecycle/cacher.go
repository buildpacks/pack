package lifecycle

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/metadata"
)

//go:generate mockgen -package testmock -destination testmock/cache.go github.com/buildpack/lifecycle Cache
type Cache interface {
	Name() string
	SetMetadata(metadata cache.Metadata) error
	RetrieveMetadata() (cache.Metadata, error)
	AddLayerFile(sha string, tarPath string) error
	ReuseLayer(sha string) error
	RetrieveLayer(sha string) (io.ReadCloser, error)
	Commit() error
}

type Cacher struct {
	ArtifactsDir string
	Buildpacks   []Buildpack
	Logger       Logger
	UID, GID     int
}

func (c *Cacher) Cache(layersDir string, cacheStore Cache) error {
	origMetadata, err := cacheStore.RetrieveMetadata()
	if err != nil {
		return errors.Wrap(err, "metadata for previous cache")
	}

	newMetadata := cache.Metadata{}
	for _, bp := range c.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, bp)
		if err != nil {
			return err
		}
		bpMetadata := metadata.BuildpackLayersMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]metadata.BuildpackLayerMetadata{},
		}
		for _, l := range bpDir.findLayers(cached) {
			if !l.hasLocalContents() {
				return fmt.Errorf("failed to cache layer '%s' because it has no contents", l.Identifier())
			}
			data, err := l.read()
			if err != nil {
				return err
			}
			origLayerMetadata := origMetadata.MetadataForBuildpack(bp.ID).Layers[l.name()]
			if data.SHA, err = c.addOrReuseLayer(cacheStore, l, origLayerMetadata.SHA); err != nil {
				return err
			}
			bpMetadata.Layers[l.name()] = data
		}
		newMetadata.Buildpacks = append(newMetadata.Buildpacks, bpMetadata)
	}

	if err := cacheStore.SetMetadata(newMetadata); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}

	return cacheStore.Commit()
}

func (c *Cacher) addOrReuseLayer(cache Cache, layer bpLayer, previousSHA string) (string, error) {
	tarPath := filepath.Join(c.ArtifactsDir, escapeID(layer.Identifier())+".tar")
	sha, err := archive.WriteTarFile(layer.Path(), tarPath, c.UID, c.GID)
	if err != nil {
		return "", errors.Wrapf(err, "caching layer '%s'", layer.Identifier())
	}

	if sha == previousSHA {
		c.Logger.Infof("Reusing layer '%s'\n", layer.Identifier())
		c.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), sha)
		return sha, cache.ReuseLayer(previousSHA)
	}

	c.Logger.Infof("Caching layer '%s'\n", layer.Identifier())
	c.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), sha)
	return sha, cache.AddLayerFile(sha, tarPath)
}
