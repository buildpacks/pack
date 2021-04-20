package asset

import (
	"archive/tar"
	"compress/gzip"
	"io/ioutil"
	"os"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/pkg/errors"

	pubcfg "github.com/buildpacks/pack/config"
	"github.com/buildpacks/pack/internal/dist"
	"github.com/buildpacks/pack/pkg/archive"
)

// NewFile is a constructor that should be used to create new File objects
func NewFile(path, os string, rawImg v1.Image, writer LayerWriter) *File {
	return &File{
		writer: writer,
		path:   path,
		os:     os,
		Image:  rawImg,
	}
}

// File contains internals needed to write an asset packages as a OCI image tarball.
type File struct {
	writer LayerWriter
	path   string
	os     string
	v1.Image
}

// Save writes an OCI image as a tarball at the path used in the NewFile constructor
// as well as each additional name.
func (a *File) Save(additionalNames ...string) error {
	tmpDir, err := ioutil.TempDir("", "create-asset-package")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if a.os == pubcfg.WindowsOS {
		if err := dist.AddWindowsShimBaseLayer(a, tmpDir); err != nil {
			// TODO -Dan- handle error
			return errors.Wrapf(err, "unable to add windows base layer")
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

// AddAssetBlobs takes a list of blobs and adds them to the File.
// Note each of these blobs must be 'openable' when Save is called.
func (a *File) AddAssetBlobs(assetBlobs ...Blob) {
	a.writer.AddAssetBlobs(assetBlobs...)
}

// SetOS sets the operating system type for the image.
// valid inputs at this time are:
// - windows
// - linux
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

// AddLayerWithDiffID adds a OCI layer tar file at path to our File object.
// Note path must be valid when Save is called.
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

// SetLabel adds a key and associated val to the OCI Label on the outside of an image.
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
