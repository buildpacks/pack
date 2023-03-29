package flatten

import (
	"archive/tar"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
)

// MergeTars merge the given tars into one single tar
func MergeTars(path string, tarsPath []string) error {
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

func addTar(tw *tar.Writer, path string) error {
	var (
		tr  *tar.Reader
		rc  io.ReadCloser
		hdr *tar.Header
		err error
	)

	if tr, rc, err = openTarFile(path); err != nil {
		return err
	}
	defer rc.Close()

	for {
		if hdr, err = tr.Next(); err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
		if err = tw.WriteHeader(hdr); err != nil {
			break
		} else if _, err = io.Copy(tw, tr); err != nil {
			break
		}
	}
	return err
}

func openTarFile(path string) (*tar.Reader, io.ReadCloser, error) {
	var (
		file  *os.File
		bytes int
		err   error
	)

	buff := make([]byte, 1024)
	if file, err = os.Open(path); err != nil {
		return nil, nil, err
	}
	if bytes, err = file.Read(buff); err != nil {
		file.Close()
		return nil, nil, err
	} else if bytes == 0 {
		file.Close()
		err = fmt.Errorf("file at path %s is empty", path)
		return nil, nil, err
	}
	if _, err = file.Seek(0, 0); err != nil {
		file.Close()
		return nil, nil, err
	}
	tr := tar.NewReader(file)
	return tr, file, nil
}
