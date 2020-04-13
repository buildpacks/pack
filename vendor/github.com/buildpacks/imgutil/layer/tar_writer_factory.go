package layer

import (
	"archive/tar"
	"io"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/archive"
)

type tarWriterFactory struct {
	os string
}

func NewTarWriterFactory(image imgutil.Image) (archive.TarWriterFactory, error) {
	os, err := image.OS()
	if err != nil {
		return nil, err
	}

	return tarWriterFactory{os: os}, nil
}

func (f tarWriterFactory) NewTarWriter(fileWriter io.Writer) archive.TarWriter {
	if f.os == "windows" {
		return NewWindowsWriter(fileWriter)
	}

	// Linux images use tar.Writer
	return tar.NewWriter(fileWriter)
}
