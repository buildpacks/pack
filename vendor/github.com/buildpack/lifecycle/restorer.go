package lifecycle

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
)

type Restorer struct {
	LayersDir  string
	Buildpacks []*Buildpack
	Out, Err   *log.Logger
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
		uid, err := strconv.Atoi(os.Getenv(cmd.EnvUID))
		if err != nil {
			return errors.Wrapf(err, "failed to convert PACK_USER_ID '%s' to int", os.Getenv(cmd.EnvUID))
		}
		gid, err := strconv.Atoi(os.Getenv(cmd.EnvGID))
		if err != nil {
			return errors.Wrapf(err, "failed to convert PACK_GROUP_ID '%s' to int", os.Getenv(cmd.EnvGID))
		}
		if err := recursiveChown(r.LayersDir, uid, gid); err != nil {
			return errors.Wrapf(err, "chowning layers dir to PACK_UID/PACK_GID '%d/%d'", uid, gid)
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

func recursiveChown(path string, uid, gid int) error {
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return err
	}
	for _, fi := range fis {
		filePath := filepath.Join(path, fi.Name())
		if fi.IsDir() {
			if err := recursiveChown(filePath, uid, gid); err != nil {
				return err
			}
		} else {
			if err := os.Chown(filePath, uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}
