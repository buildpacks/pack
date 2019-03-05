package fs

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type FS struct {
}

func (*FS) CreateTarFile(tarFile, srcDir, tarDir string, uid, gid int) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()
	return writeTarArchive(fh, srcDir, tarDir, uid, gid)
}

func (*FS) CreateTarReader(srcDir, tarDir string, uid, gid int) (io.Reader, chan error) {
	r, w := io.Pipe()
	errChan := make(chan error, 1)

	go func() {
		defer w.Close()
		err := writeTarArchive(w, srcDir, tarDir, uid, gid)
		w.Close()
		errChan <- err
	}()
	return r, errChan
}

func (*FS) CreateSingleFileTarReader(path, txt string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(txt)), Mode: 0666}); err != nil {
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

func (*FS) CreateSingleFileTar(tarFile, path, txt string) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}

	tw := tar.NewWriter(fh)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(txt)), Mode: 0666}); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(txt)); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return nil
}

func writeParentDirectoryHeaders(tarDir string, tw *tar.Writer, uid int, gid int) error {
	parent := filepath.Dir(tarDir)
	if parent != "." && parent != "/" {
		if err := writeParentDirectoryHeaders(parent, tw, uid, gid); err != nil {
			return err
		}
	}
	header := &tar.Header{
		Name:     tarDir,
		Uid:      uid,
		Gid:      gid,
		Mode:     0755,
		Typeflag: tar.TypeDir,
		ModTime:  time.Time{},
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	return nil
}

func writeTarArchive(w io.Writer, srcDir, tarDir string, uid, gid int) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	err := writeParentDirectoryHeaders(tarDir, tw, uid, gid)
	if err != nil {
		return err
	}

	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
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
		}
		if relPath == "." {
			return nil
		}
		header.Name = filepath.Join(tarDir, relPath)
		if runtime.GOOS == "windows" {
			header.Name = strings.Replace(header.Name, "\\", "/", -1)
		}
		header.ModTime = time.Time{}
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

func (*FS) AddTextToTar(tw *tar.Writer, name string, contents []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(contents)
	return err
}

func (*FS) AddFileToTar(tw *tar.Writer, name string, contents *os.File) error {
	fi, err := contents.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(fi.Size())}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, contents)
	return err
}

func (*FS) Untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			return nil
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			_, err := os.Stat(filepath.Dir(path))
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
			}

			fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(fh, tr); err != nil {
				fh.Close()
				return err
			}
			fh.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type in tar %d", hdr.Typeflag)
		}
	}
}
