package buildpack

import (
	"archive/tar"
	"os"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/pkg/errors"

	"github.com/buildpacks/pack/pkg/archive"
	"github.com/buildpacks/pack/pkg/dist"
)

// FilePackageBuilder collects platform-specific packages in one OCI layout.
// Close releases its temporary files after the package has been saved.
type FilePackageBuilder struct {
	directory string
}

func NewFilePackageBuilder() (*FilePackageBuilder, error) {
	directory, err := os.MkdirTemp("", "pack-file-package-")
	if err != nil {
		return nil, errors.Wrap(err, "creating package directory")
	}
	if _, err := layout.Write(directory, empty.Index); err != nil {
		os.RemoveAll(directory)
		return nil, errors.Wrap(err, "initializing package layout")
	}
	return &FilePackageBuilder{directory: directory}, nil
}

// Add stores the package and its platform descriptor, sharing identical blobs.
func (p *FilePackageBuilder) Add(builder *PackageBuilder, target dist.Target, labels map[string]string) error {
	if p.directory == "" {
		return errors.New("file package builder is closed")
	}
	return builder.saveAsLayout(p.directory, target, labels)
}

// Save writes all collected platforms to a single .cnb file.
func (p *FilePackageBuilder) Save(filename string) (err error) {
	if p.directory == "" {
		return errors.New("file package builder is closed")
	}
	output, err := os.Create(filename)
	if err != nil {
		return errors.Wrap(err, "creating output file")
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}()
	writer := tar.NewWriter(output)
	defer func() {
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
	}()
	return archive.WriteDirToTar(writer, p.directory, "/", 0, 0, 0755, true, false, nil)
}

func (p *FilePackageBuilder) Close() error {
	if p.directory == "" {
		return nil
	}
	err := os.RemoveAll(p.directory)
	p.directory = ""
	return err
}
