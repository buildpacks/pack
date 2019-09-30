package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/pkg/errors"
)

var NormalizedDateTime time.Time

func init() {
	NormalizedDateTime = time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)
}

func ReadDirAsTar(srcDir, basePath string, uid, gid int, mode int64) io.ReadCloser {
	return readAsTar(srcDir, basePath, uid, gid, mode, WriteDirToTar)
}

func ReadZipAsTar(srcPath, basePath string, uid, gid int, mode int64) io.ReadCloser {
	return readAsTar(srcPath, basePath, uid, gid, mode, WriteZipToTar)
}

func readAsTar(src, basePath string, uid, gid int, mode int64, writeFn func(tw *tar.Writer, srcDir, basePath string, uid, gid int, mode int64) error) io.ReadCloser {
	var (
		errChan = make(chan error)
		r, w    = io.Pipe()
	)

	go func() {
		tw := tar.NewWriter(w)
		defer func() {
			if r := recover(); r != nil {
				tw.Close()
				w.CloseWithError(errors.Errorf("panic: %v", r))
			}
		}()

		err := writeFn(tw, src, basePath, uid, gid, mode)

		closeErr := tw.Close()
		closeErr = aggregateError(closeErr, w.CloseWithError(err))

		errChan <- closeErr
	}()

	return ioutils.NewReadCloserWrapper(r, func() error {
		var compErr error

		// closing the reader ensures that if anything attempts
		// further reading it doesn't block waiting for content
		if err := r.Close(); err != nil {
			compErr = aggregateError(compErr, err)
		}

		// wait until everything closes properly specially contents inside `writeFn`
		if err := <-errChan; err != nil {
			compErr = aggregateError(compErr, err)
		}

		return compErr
	})
}

func aggregateError(base, addition error) error {
	if addition == nil {
		return base
	}

	if base == nil {
		return addition
	}

	return errors.Wrap(addition, base.Error())
}

func CreateSingleFileTarReader(path, txt string) (io.Reader, error) {
	var buf bytes.Buffer
	tarBuilder := TarBuilder{}
	tarBuilder.AddFile(path, 0644, []byte(txt))
	_, err := tarBuilder.WriteTo(&buf)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(buf.Bytes()), nil
}

func CreateSingleFileTar(tarFile, path, txt string) error {
	tarBuilder := TarBuilder{}
	tarBuilder.AddFile(path, 0644, []byte(txt))
	return tarBuilder.WriteToPath(tarFile)
}

func AddFileToTar(tw *tar.Writer, path string, txt string) error {
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
	return nil
}

var ErrEntryNotExist = errors.New("not exist")

func ReadTarEntry(rc io.Reader, entryPath string) (*tar.Header, []byte, error) {
	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to get next tar entry")
		}

		if path.Clean(header.Name) == entryPath {
			buf, err := ioutil.ReadAll(tr)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to read contents of '%s'", entryPath)
			}

			return header, buf, nil
		}
	}

	return nil, nil, errors.Wrapf(ErrEntryNotExist, "could not find entry path '%s'", entryPath)
}

func WriteDirToTar(tw *tar.Writer, srcDir, basePath string, uid, gid int, mode int64) error {
	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.Mode()&os.ModeSocket != 0 {
			return nil
		}

		var header *tar.Header
		if fi.Mode()&os.ModeSymlink != 0 {
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

		header.Name = filepath.ToSlash(filepath.Join(basePath, relPath))
		finalizeHeader(header, uid, gid, mode)

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

func WriteZipToTar(tw *tar.Writer, srcZip, basePath string, uid, gid int, mode int64) error {
	zipReader, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		var header *tar.Header
		if f.Mode()&os.ModeSymlink != 0 {
			target, err := func() (string, error) {
				r, err := f.Open()
				if err != nil {
					return "", nil
				}
				defer r.Close()

				// contents is the target of the symlink
				target, err := ioutil.ReadAll(r)
				if err != nil {
					return "", err
				}

				return string(target), nil
			}()

			if err != nil {
				return err
			}

			header, err = tar.FileInfoHeader(f.FileInfo(), target)
			if err != nil {
				return err
			}
		} else {
			header, err = tar.FileInfoHeader(f.FileInfo(), f.Name)
			if err != nil {
				return err
			}
		}

		header.Name = filepath.ToSlash(filepath.Join(basePath, f.Name))
		finalizeHeader(header, uid, gid, mode)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if f.Mode().IsRegular() {
			err := func() error {
				fi, err := f.Open()
				if err != nil {
					return err
				}
				defer fi.Close()

				_, err = io.Copy(tw, fi)
				return err
			}()

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func finalizeHeader(header *tar.Header, uid, gid int, mode int64) {
	if mode != -1 {
		header.Mode = mode
	}
	header.ModTime = NormalizedDateTime
	header.Uid = uid
	header.Gid = gid
	header.Uname = ""
	header.Gname = ""
}

func IsZip(file io.Reader) (bool, error) {
	b := make([]byte, 4)
	_, err := file.Read(b)
	if err != nil && err != io.EOF {
		return false, err
	} else if err == io.EOF {
		return false, nil
	}

	return bytes.Equal(b, []byte("\x50\x4B\x03\x04")), nil
}
