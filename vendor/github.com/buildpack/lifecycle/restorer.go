package lifecycle

import (
	"os"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/metadata"
)

type Restorer struct {
	LayersDir  string
	Buildpacks []Buildpack
	Logger     Logger
	UID        int
	GID        int
}

func (r *Restorer) Restore(cache Cache) error {
	meta, err := cache.RetrieveMetadata()
	if err != nil {
		return err
	}

	if len(meta.Buildpacks) == 0 {
		r.Logger.Infof("Cache '%s': metadata not found, nothing to restore", cache.Name())
		return nil
	}

	for _, bp := range r.Buildpacks {
		layersDir, err := readBuildpackLayersDir(r.LayersDir, bp)
		if err != nil {
			return err
		}
		bpMD := meta.MetadataForBuildpack(bp.ID)
		for name, layer := range bpMD.Layers {
			if !layer.Cache {
				continue
			}

			if err := r.restoreLayer(name, bpMD, layer, layersDir, cache); err != nil {
				return err
			}
		}
	}

	// if restorer is running as root it needs to fix the ownership of the layers dir
	if current := os.Getuid(); current == -1 {
		return errors.New("cannot determine UID")
	} else if current == 0 {
		if err := recursiveChown(r.LayersDir, r.UID, r.GID); err != nil {
			return errors.Wrapf(err, "chowning layers dir to '%d/%d'", r.UID, r.GID)
		}
	}
	return nil
}

func (r *Restorer) restoreLayer(name string, bpMD metadata.BuildpackLayersMetadata, layer metadata.BuildpackLayerMetadata, layersDir bpLayersDir, cache Cache) error {
	bpLayer := layersDir.newBPLayer(name)

	r.Logger.Infof("Restoring cached layer '%s'", bpLayer.Identifier())
	if err := bpLayer.writeMetadata(bpMD.Layers); err != nil {
		return err
	}

	if layer.Launch {
		if err := bpLayer.writeSha(layer.SHA); err != nil {
			return err
		}
	}

	rc, err := cache.RetrieveLayer(layer.SHA)
	if err != nil {
		return err
	}
	defer rc.Close()

	return archive.Untar(rc, "/")
}
