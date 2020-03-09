package registry

import (
	"os"
	"path/filepath"

	"github.com/buildpacks/pack/internal/config"
)

const defaultRegistryURL = "https://github.com/jkutner/buildpack-registry"

const defaultRegistyDir = "registry"

type Buildpack struct {
	Namespace string `json:"ns"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Yanked    bool   `json:"yanked"`
	Digest    bool   `json:"digest"`
	Address   string `json:"addr"`
}

type Entry struct {
	Buildpacks []Buildpack `json:"buildpacks"`
}

type RegistryCache struct {
	url  string
	path string
}

func NewRegistryCache() (RegistryCache, error) {
	home, err := config.PackHome()
	if err != nil {
		return RegistryCache{}, err
	}

	r := RegistryCache{
		url:  defaultRegistryURL,
		path: filepath.Join(home, defaultRegistyDir),
	}
	return r, r.initialize()
}

func (r *RegistryCache) initialize() error {
	_, err := os.Stat(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			// git clone
			return nil
		}
	}
	return err
}

func (r *RegistryCache) refresh() error {
	// git pull
	return nil
}

func (r *RegistryCache) LocateBuildpack(bp string) (Buildpack, error) {
	r.refresh()
	// parse the bp string
	// find the file xx/yy/ns_bp
	// read the JSON
	// get the right version from JSON
	// get the docker/image URI from JSON

	return Buildpack{}, nil
}
