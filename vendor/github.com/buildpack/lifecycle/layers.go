package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type bpLayersDir struct {
	path      string
	layers    map[string]bpLayer
	name      string
	buildpack Buildpack
}

func readBuildpackLayersDir(layersDir string, buildpack Buildpack) (bpLayersDir, error) {
	path := filepath.Join(layersDir, buildpack.EscapedID())
	bpDir := bpLayersDir{
		name:      buildpack.ID,
		path:      path,
		layers:    map[string]bpLayer{},
		buildpack: buildpack,
	}

	fis, err := ioutil.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		return bpDir, err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			bpDir.layers[fi.Name()] = *bpDir.newBPLayer(fi.Name())
		}
	}

	tomls, err := filepath.Glob(filepath.Join(path, "*.toml"))
	for _, toml := range tomls {
		name := strings.TrimRight(filepath.Base(toml), ".toml")
		bpDir.layers[name] = *bpDir.newBPLayer(name)
	}
	return bpDir, nil
}

func launch(l bpLayer) bool {
	md, err := l.read()
	return err == nil && md.Launch
}

func nonCached(l bpLayer) bool {
	md, err := l.read()
	return err == nil && !md.Cache
}

func malformed(l bpLayer) bool {
	_, err := l.read()
	return err != nil
}

func cached(l bpLayer) bool {
	md, err := l.read()
	return err == nil && md.Cache
}

func (bd *bpLayersDir) findLayers(f func(layer bpLayer) bool) []bpLayer {
	var selectedLayers []bpLayer
	for _, l := range bd.layers {
		if f(l) {
			selectedLayers = append(selectedLayers, l)
		}
	}
	return selectedLayers
}

func (bd *bpLayersDir) newBPLayer(name string) *bpLayer {
	return &bpLayer{
		layer{
			path:       filepath.Join(bd.path, name),
			identifier: fmt.Sprintf("%s:%s", bd.buildpack.ID, name),
		},
	}
}

type cacheType int

const (
	cacheStaleNoMetadata cacheType = iota
	cacheStaleWrongSHA
	cacheNotForLaunch // we can't determine whether the cache is stale for launch=false layers
	cacheValid
	cacheMalformed
)

type bpLayer struct {
	layer
}

func (bp *bpLayer) classifyCache(metadataLayers map[string]LayerMetadata) cacheType {
	cachedLayer, err := bp.read()
	if err != nil {
		return cacheMalformed
	}
	if !cachedLayer.Launch {
		return cacheNotForLaunch
	}
	if metadataLayers == nil {
		return cacheStaleNoMetadata
	}
	layerMetadata, ok := metadataLayers[bp.name()]
	if !ok {
		return cacheStaleNoMetadata
	}
	if layerMetadata.SHA != cachedLayer.SHA {
		return cacheStaleWrongSHA
	}
	return cacheValid
}

func (bp *bpLayer) read() (LayerMetadata, error) {
	var metadata LayerMetadata
	tomlPath := bp.path + ".toml"
	fh, err := os.Open(tomlPath)
	if os.IsNotExist(err) {
		return LayerMetadata{}, nil
	} else if err != nil {
		return LayerMetadata{}, err
	}
	defer fh.Close()
	if _, err := toml.DecodeFile(tomlPath, &metadata); err != nil {
		return LayerMetadata{}, err
	}
	sha, err := ioutil.ReadFile(bp.path + ".sha")
	if err != nil {
		if os.IsNotExist(err) {
			return metadata, nil
		} else {
			return LayerMetadata{}, err
		}
	}
	metadata.SHA = string(sha)
	return metadata, nil
}

func (bp *bpLayer) remove() error {
	if err := os.RemoveAll(bp.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".sha"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".toml"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (bp *bpLayer) writeMetadata(metadataLayers map[string]LayerMetadata) error {
	layerMetadata := metadataLayers[bp.name()]
	path := filepath.Join(bp.path + ".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return toml.NewEncoder(fh).Encode(layerMetadata)
}

func (bp *bpLayer) hasLocalContents() bool {
	_, err := ioutil.ReadDir(bp.path)

	return !os.IsNotExist(err)
}

func (bp *bpLayer) writeSha(sha string) error {
	if err := ioutil.WriteFile(filepath.Join(bp.path+".sha"), []byte(sha), 0777); err != nil {
		return err
	}
	return nil
}

func (bp *bpLayer) name() string {
	return filepath.Base(bp.path)
}

type layer struct {
	path       string
	identifier string
}

func (l *layer) Identifier() string {
	return l.identifier
}

func (l *layer) Path() string {
	return l.path
}

type identifiableLayer interface {
	Identifier() string
	Path() string
}
