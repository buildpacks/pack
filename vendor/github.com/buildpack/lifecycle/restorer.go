package lifecycle

import (
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/image"
)

type Restorer struct {
	LayersDir  string
	Buildpacks []*Buildpack
	Out, Err   *log.Logger
	UID        int
	GID        int
}

func (r *Restorer) Restore(cacheImage image.Image) error {
	if found, err := cacheImage.Found(); !found || err != nil {
		r.Out.Printf("cache image '%s' not found, nothing to restore", cacheImage.Name())
		return nil
	}
	metadata, err := getCacheMetadata(cacheImage, r.Out)
	if err != nil {
		return err
	}
	for _, bp := range r.Buildpacks {
		layersDir, err := readBuildpackLayersDir(r.LayersDir, *bp)
		if err != nil {
			return err
		}
		bpMD := metadata.metadataForBuildpack(bp.ID)
		for name, layer := range bpMD.Layers {
			if !layer.Cache {
				continue
			}

			if err := r.restoreLayer(name, bpMD, layer, layersDir, cacheImage); err != nil {
				return err
			}
		}
	}

	// if restorer is running as root it needs to fix the ownership of the layers dir
	if current := os.Getuid(); err != nil {
		return err
	} else if current == 0 {
		if err := recursiveChown(r.LayersDir, r.UID, r.GID); err != nil {
			return errors.Wrapf(err, "chowning layers dir to '%d/%d'", r.UID, r.GID)
		}
	}
	return nil
}

func (r *Restorer) restoreLayer(name string, bpMD BuildpackMetadata, layer LayerMetadata, layersDir bpLayersDir, cacheImage image.Image) error {
	bpLayer := layersDir.newBPLayer(name)

	r.Out.Printf("restoring cached layer '%s'", bpLayer.Identifier())
	if err := bpLayer.writeMetadata(bpMD.Layers); err != nil {
		return err
	}

	if layer.Launch {
		if err := bpLayer.writeSha(layer.SHA); err != nil {
			return err
		}
	}

	rc, err := cacheImage.GetLayer(layer.SHA)
	if err != nil {
		return err
	}
	defer rc.Close()

	return archive.Untar(rc, "/")
}
