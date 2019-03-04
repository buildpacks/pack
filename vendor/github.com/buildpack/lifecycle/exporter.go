package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/fs"
	"github.com/buildpack/lifecycle/image"
)

type Exporter struct {
	Buildpacks   []*Buildpack
	ArtifactsDir string
	In           []byte
	Out, Err     *log.Logger
	UID, GID     int
}

func (e *Exporter) Export(layersDir, appDir string, runImage, origImage image.Image, launcher string) error {
	var err error
	metadata := AppImageMetadata{}
	metadata.RunImage.TopLayer, err = runImage.TopLayer()
	if err != nil {
		return errors.Wrap(err, "get run image top layer SHA")
	}
	metadata.RunImage.SHA, err = runImage.Digest()
	if err != nil {
		return errors.Wrap(err, "get run image digest")
	}

	origMetadata, err := e.getImageMetadata(origImage)
	if err != nil {
		return errors.Wrap(err, "metadata for previous image")
	}

	runImage.Rename(origImage.Name())
	appImage := &loggingImage{
		Out:   e.Out,
		image: runImage,
	}

	metadata.App.SHA, err = e.addOrReuseLayer(appImage, &layer{path: appDir, identifier: "app"}, origMetadata.App.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting app layer")
	}

	metadata.Config.SHA, err = e.addOrReuseLayer(appImage, &layer{path: filepath.Join(layersDir, "config"), identifier: "config"}, origMetadata.Config.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting config layer")
	}

	metadata.Launcher.SHA, err = e.addOrReuseLayer(appImage, &layer{path: launcher, identifier: "launcher"}, origMetadata.Launcher.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting launcher layer")
	}

	for _, bp := range e.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, bp.ID)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}
		bpMD := BuildpackMetadata{ID: bp.ID, Layers: map[string]LayerMetadata{}}

		for _, layer := range bpDir.findLayers(launch) {
			lmd, err := layer.read()
			if err != nil {
				return errors.Wrapf(err, "reading '%s' metadata", layer.Identifier())
			}

			if layer.hasLocalContents() {
				origLayerMetadata := origMetadata.metadataForBuildpack(bp.ID).Layers[layer.name()]
				lmd.SHA, err = e.addOrReuseLayer(appImage, &layer, origLayerMetadata.SHA)
				if err != nil {
					return err
				}

				if err := layer.writeSha(lmd.SHA); err != nil {
					return errors.Wrapf(err, "writing '%s' sha", layer.Identifier())
				}
			} else {
				origLayerMetadata, ok := origMetadata.metadataForBuildpack(bp.ID).Layers[layer.name()]
				if !ok {
					return fmt.Errorf("cannot reuse '%s', previous image has no metadata for layer '%s'", layer.Identifier(), layer.Identifier())
				}

				if err := appImage.ReuseLayer(layer.Identifier(), origLayerMetadata.SHA); err != nil {
					return errors.Wrapf(err, "reusing layer: '%s'", layer.Identifier())
				}
				lmd.SHA = origLayerMetadata.SHA

				if err := layer.remove(); err != nil {
					return errors.Wrapf(err, "removing layer: '%s'", layer.Identifier())
				}
			}
			bpMD.Layers[layer.name()] = lmd
		}

		for _, layer := range bpDir.findLayers(nonCached) {
			if err := layer.remove(); err != nil {
				return errors.Wrapf(err, "removing non-cached layer: '%s'", layer.Identifier())
			}
		}

		if malformedLayers := bpDir.findLayers(malformed); len(malformedLayers) > 0 {
			ids := make([]string, 0, len(malformedLayers))
			for _, ml := range malformedLayers {
				ids = append(ids, ml.identifier)
			}
			return fmt.Errorf("failed to parse metadata for layers '%s'", ids)
		}

		metadata.Buildpacks = append(metadata.Buildpacks, bpMD)
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	if err := appImage.SetLabel(MetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	if err := appImage.SetEnv(cmd.EnvLayersDir, layersDir); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	if err := appImage.SetEnv(cmd.EnvAppDir, appDir); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	if err := appImage.SetEntrypoint(launcher); err != nil {
		return errors.Wrap(err, "setting entrypoint")
	}
	if err := appImage.SetEmptyCmd(); err != nil {
		return errors.Wrap(err, "setting cmd")
	}
	if err := e.cleanBuildpacksNotInGroup(layersDir); err != nil {
		return errors.Wrap(err, "failed to cleanup layers dir")
	}
	_, err = appImage.Save()
	return err
}

func (e *Exporter) addOrReuseLayer(image *loggingImage, layer identifiableLayer, previousSha string) (string, error) {
	sha, err := e.exportTar(layer.Path())
	if err != nil {
		return "", errors.Wrapf(err, "exporting layer '%s'", layer.Identifier())
	}
	if sha == previousSha {
		return sha, image.ReuseLayer(layer.Identifier(), previousSha)
	}
	return sha, image.AddLayer(layer.Identifier(), sha, e.tarPath(sha))
}

func (e *Exporter) tarPath(sha string) string {
	return filepath.Join(e.ArtifactsDir, strings.TrimPrefix(sha, "sha256:")+".tar")
}

func (e *Exporter) cleanBuildpacksNotInGroup(layersDir string) error {
	fis, err := ioutil.ReadDir(layersDir)
	if err != nil {
		return err
	}
	for _, fi := range fis {
		if e.groupContainsBuildpack(fi.Name()) {
			continue
		}

		if err := os.RemoveAll(filepath.Join(layersDir, fi.Name())); err != nil {
			return errors.Wrap(err, "failed to cleanup layers dir")
		}
	}
	return nil
}

func (e *Exporter) groupContainsBuildpack(name string) bool {
	for _, buildpack := range e.Buildpacks {
		if name == buildpack.EscapedID() {
			return true
		}
	}
	return false
}

func (e *Exporter) getImageMetadata(image image.Image) (AppImageMetadata, error) {
	var metadata AppImageMetadata
	found, err := image.Found()
	if err != nil {
		return metadata, errors.Wrap(err, "looking for image")
	}
	if found {
		label, err := image.Label(MetadataLabel)
		if err != nil {
			return metadata, errors.Wrap(err, "getting metadata")
		}
		if err := json.Unmarshal([]byte(label), &metadata); err != nil {
			return metadata, err
		}
	}
	return metadata, nil
}

func (e *Exporter) exportTar(sourceDir string) (string, error) {
	hasher := sha256.New()
	f, err := ioutil.TempFile(e.ArtifactsDir, "tarfile")
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := io.MultiWriter(hasher, f)

	fs := &fs.FS{}
	err = fs.WriteTarArchive(w, sourceDir, e.UID, e.GID)
	if err != nil {
		return "", err
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(e.ArtifactsDir, sha+".tar")); err != nil {
		return "", err
	}

	return "sha256:" + sha, nil
}
