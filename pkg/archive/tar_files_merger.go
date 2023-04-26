package archive

import (
	"archive/tar"
	"io"
	"os"

	"github.com/pkg/errors"
)

// MergeTars merge the given tars into one single tar
func MergeTars(path string, tarsPath ...string) error {
	tarFile, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "creating flatten tar")
	}
	defer tarFile.Close()
	tw := tar.NewWriter(tarFile)

	for _, tarPath := range tarsPath {
		if err := addTar(tw, tarPath); err != nil {
			return errors.Wrap(err, "adding tar")
		}
	}
	if err := tw.Close(); err != nil {
		return errors.Wrap(err, "Failed to close tar writer")
	}
	return nil
}

// addTar writes the content of the given tar file into the writer
func addTar(tw *tar.Writer, path string) error {
	var (
		reader *tar.Reader
		rc     io.ReadCloser
		header *tar.Header
		err    error
	)

	if reader, rc, err = openTarFile(path); err != nil {
		return err
	}
	defer rc.Close()

	for {
		if header, err = reader.Next(); err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
		if header.FileInfo().Size() == 0 {
			break
		}

		if err = tw.WriteHeader(header); err != nil {
			break
		} else if _, err = io.Copy(tw, reader); err != nil {
			break
		}
	}
	return err
}

// openTarFile opens the given tar file and returns a reader and a closer
func openTarFile(path string) (*tar.Reader, io.ReadCloser, error) {
	var (
		rc  io.ReadCloser
		err error
	)

	if rc, err = os.Open(path); err != nil {
		return nil, nil, err
	}
	return tar.NewReader(rc), rc, nil
}
