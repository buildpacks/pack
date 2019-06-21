package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pkg/errors"
)

var NormalizedDateTime time.Time

func init() {
	NormalizedDateTime = time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)
}

func CreateTarReader(srcDir, tarDir string, uid, gid int, mode int64) (io.Reader, chan error) {
	r, w := io.Pipe()
	errChan := make(chan error, 1)
	go func() {
		defer w.Close()

		tw := tar.NewWriter(w)
		defer tw.Close()

		err := WriteDirToTar(tw, srcDir, tarDir, uid, gid, mode)
		errChan <- err
	}()
	return r, errChan
}

func CreateSingleFileTarReader(path, txt string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: path,
		Size: int64(len(txt)),
		Mode: 0644,
	}); err != nil {
		return nil, err
	}

	if _, err := tw.Write([]byte(txt)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return bytes.NewReader(buf.Bytes()), nil
}

func CreateSingleFileTar(tarFile, path, txt string) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	if err := tw.WriteHeader(&tar.Header{
		Name: path,
		Size: int64(len(txt)),
		Mode: 0644,
	}); err != nil {
		return err
	}

	if _, err := tw.Write([]byte(txt)); err != nil {
		return err
	}

	return tw.Close()
}

func ReadTarEntry(tarPath string, entryPath ...string) (*tar.Header, []byte, error) {
	var (
		tarFile    *os.File
		gzipReader *gzip.Reader
		fhFinal    io.Reader
		err        error
	)

	tarFile, err = os.Open(tarPath)
	fhFinal = tarFile
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to open tar '%s'", tarPath)
	}
	defer tarFile.Close()

	if filepath.Ext(tarPath) == ".tgz" {
		gzipReader, err = gzip.NewReader(tarFile)
		fhFinal = gzipReader
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to create gzip reader")
		}

		defer gzipReader.Close()
	}

	tr := tar.NewReader(fhFinal)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to get next tar entry")
		}

		if contains(entryPath, header.Name) {
			buf, err := ioutil.ReadAll(tr)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to read contents of '%s'", entryPath)
			}

			return header, buf, nil
		}
	}

	return nil, nil, fmt.Errorf("could not find entry path '%s' in tar", entryPath)
}

func contains(slice []string, element string) bool {
	for _, a := range slice {
		if a == element {
			return true
		}
	}
	return false
}

func WriteDirToTar(tw *tar.Writer, srcDir, tarDir string, uid, gid int, mode int64) error {
	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isSymlink := fi.Mode()&os.ModeSymlink != 0

		if !fi.Mode().IsRegular() && !fi.IsDir() && !isSymlink {
			return nil
		}

		var header *tar.Header
		if isSymlink {
			target, err := os.Readlink(file)
			if err != nil {
				return err
			}

			header, err = tar.FileInfoHeader(fi, target)
			if err != nil {
				return err
			}
		} else {
			header, err = tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}
		}

		relPath, err := filepath.Rel(srcDir, file)
		if err != nil {
			return err
		} else if relPath == "." {
			return nil
		}

		header.Name = filepath.Join(tarDir, relPath)
		if runtime.GOOS == "windows" {
			header.Name = filepath.ToSlash(header.Name)
		}
		if mode != -1 {
			header.Mode = mode
		}
		header.ModTime = NormalizedDateTime
		header.Uid = uid
		header.Gid = gid
		header.Uname = ""
		header.Gname = ""

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if fi.Mode().IsRegular() {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}

		return nil
	})
}
