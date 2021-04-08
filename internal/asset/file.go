package asset

import (
	"archive/tar"
	"compress/gzip"
	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
)

// TODO: much of this implementation should be replaced when imgutil can save
//   images as archives.
type SaveableCacheImage interface {
	dist.WorkableImage
	Save(assetWriter layerWriter, additionalName ...string) error
}

func NewFile(path, os string, rawImg v1.Image, writer LayerWriter) *File {
	return &File{
		writer:         writer,
		path:           path,
		os:             os,
		assetsAndBlobs: []BlobAssetPair{},
		Image:          rawImg,
	}
}

type File struct {
	writer         LayerWriter
	path           string
	os             string
	assetsAndBlobs []BlobAssetPair
	v1.Image
}

func (a *File) Save(additionalNames ...string) error {
	tmpDir, err := ioutil.TempDir("", "create-asset-cache")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if a.os == pubcfg.WindowsOS {
		if err := dist.AddWindowsShimBaseLayer(a, tmpDir); err != nil {
			// TODO -Dan- handle error
			panic(err)
		}
	}

	if err := a.writer.Open(); err != nil {
		return errors.Wrap(err, "unable to open asset writer")
	}
	defer a.writer.Close()

	err = a.writer.Write(a)
	if err != nil {
		return errors.Wrap(err, "unable to write asset layers to file")
	}

	layoutDir, err := ioutil.TempDir(tmpDir, "oci-layout")
	if err != nil {
		return errors.Wrap(err, "creating oci-layout temp dir")
	}

	p, err := layout.Write(layoutDir, empty.Index)
	if err != nil {
		return errors.Wrap(err, "writing index")
	}

	if err := p.AppendImage(a); err != nil {
		return errors.Wrap(err, "writing layout")
	}

	for _, path := range append([]string{a.path}, additionalNames...) {
		outputFile, err := os.Create(path)
		if err != nil {
			return errors.Wrap(err, "creating output file")
		}
		defer outputFile.Close()

		tw := tar.NewWriter(outputFile)
		defer tw.Close()

		err = archive.WriteDirToTar(tw, layoutDir, "/", 0, 0, 0755, true, nil)
		if err != nil {
			return errors.Wrapf(err, "error writing image asset image to file")
		}
	}
	return nil
}

func (a *File) AddAssetBlobs(layerBlobs ...Blob) {
	a.writer.AddAssetBlobs(layerBlobs...)
}

func (a *File) SetOS(osVal string) error {
	a.os = osVal
	configFile, err := a.ConfigFile()
	if err != nil {
		return err
	}
	configFile.OS = osVal
	a.Image, err = mutate.ConfigFile(a.Image, configFile)
	return err
}

func (a *File) AddLayerWithDiffID(path, _ string) error {
	tarLayer, err := tarball.LayerFromFile(path, tarball.WithCompressionLevel(gzip.DefaultCompression))
	if err != nil {
		return err
	}
	a.Image, err = mutate.AppendLayers(a.Image, tarLayer)
	if err != nil {
		return errors.Wrap(err, "add layer")
	}
	return nil
}

func (a *File) SetLabel(key string, val string) error {
	configFile, err := a.ConfigFile()
	if err != nil {
		return err
	}
	config := *configFile.Config.DeepCopy()
	if config.Labels == nil {
		config.Labels = map[string]string{}
	}
	config.Labels[key] = val
	a.Image, err = mutate.Config(a.Image, config)
	return err
}
